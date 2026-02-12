/**
 * api_bridge.js — Универсальный API мост для TeleGhost.
 *
 * Desktop (Wails):  window.go.main.App.<Method>(args)
 * Mobile  (Android): POST http://127.0.0.1:8080/api/<method> { args: [...] }
 *
 * Экспортируемый объект `Api` содержит промис-обёртки для каждого метода.
 * Пример использования:
 *   import { Api } from '$lib/api_bridge.js';
 *   const profiles = await Api.ListProfiles();
 *   await Api.CreateProfile("Alice", "123456", "", "", "", true);
 */

const MOBILE_BASE_URL = 'http://127.0.0.1:8080/api';

/**
 * Определяем среду выполнения.
 * На Desktop Wails инжектит `window.go` с биндингами.
 * На Android WebView этого объекта нет — шлём HTTP.
 */
function isDesktop() {
    try {
        return !!(window.go && window.go.main && window.go.main.App);
    } catch {
        return false;
    }
}

/**
 * Отправляет HTTP-запрос к Go HTTP серверу на localhost (для мобилки).
 * @param {string} method  — имя метода (PascalCase, как в Go)
 * @param  {...any} args   — аргументы метода
 * @returns {Promise<any>} — результат из JSON-ответа
 */
async function callMobile(method, ...args) {
    const url = `${MOBILE_BASE_URL}/${method}`;

    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ args }),
    });

    if (!resp.ok) {
        const errText = await resp.text().catch(() => resp.statusText);
        throw new Error(`API ${method}: ${resp.status} — ${errText}`);
    }

    const json = await resp.json();

    if (json.error) {
        throw new Error(json.error);
    }

    return json.result;
}

/**
 * Создаёт proxy-обёртку: если мы на десктопе — дёргаем Wails,
 * иначе — HTTP.
 * @param {string} method
 * @returns {Function}
 */
function bridgeMethod(method) {
    return async (...args) => {
        if (isDesktop()) {
            // Wails binding: window.go.main.App.MethodName(arg1, arg2, ...)
            return window.go.main.App[method](...args);
        }
        return callMobile(method, ...args);
    };
}

// ─── Полный перечень методов App-структуры ──────────────────────────────────

const METHODS = [
    // === Auth ===
    'CreateProfile',
    'UpdateProfile',
    'ListProfiles',
    'UnlockProfile',
    'DeleteProfile',
    'Login',
    'CreateAccount',
    'Logout',
    'GetMyInfo',
    'GetCurrentProfile',
    'UpdateMyProfile',
    'RequestProfileUpdate',

    // === Contacts ===
    'AddContact',
    'AddContactFromClipboard',
    'DeleteContact',
    'GetContacts',

    // === Folders ===
    'CreateFolder',
    'GetFolders',
    'DeleteFolder',
    'UpdateFolder',
    'AddChatToFolder',
    'RemoveChatFromFolder',

    // === Messages ===
    'SendText',
    'SendFileMessage',
    'GetMessages',
    'EditMessage',
    'DeleteMessage',
    'DeleteMessageForAll',
    'AcceptFileTransfer',
    'DeclineFileTransfer',

    // === Settings ===
    'GetMyDestination',
    'GetRouterSettings',
    'SaveRouterSettings',
    'GetAppAboutInfo',
    'CheckForUpdates',
    'GetNetworkStatus',

    // === Reseed ===
    'ExportReseed',
    'ImportReseed',

    // === Utils ===
    'GetFileBase64',
    'SaveTempImage',
    'SelectFiles',
    'SelectImage',
    'OpenFile',
    'ShowInFolder',
    'SaveFileToLocation',
    'CopyToClipboard',
    'CopyImageToClipboard',
    'GetImageThumbnail',

    // === Account Backup ===
    'ExportAccount',
    'ImportAccount',

    // === Notifications ===
    'GetUnreadCount',
    'MarkChatAsRead',
];

// ─── Генерируем объект Api ──────────────────────────────────────────────────

/** @type {Record<string, Function>} */
export const Api = {};

for (const m of METHODS) {
    Api[m] = bridgeMethod(m);
}

// ─── EventBus для мобилки (заглушка SSE / polling) ──────────────────────────

/**
 * На Desktop Wails даёт runtime.EventsOn().
 * На Mobile мы стартуем SSE-стрим с сервера для real-time событий.
 *
 * Использование:
 *   import { EventBridge } from '$lib/api_bridge.js';
 *   EventBridge.on('new_message', (data) => { ... });
 */
class MobileEventBridge {
    constructor() {
        /** @type {Map<string, Function[]>} */
        this._listeners = new Map();
        this._evtSource = null;
    }

    /**
     * Подписка на событие.
     * @param {string} event
     * @param {Function} callback
     */
    on(event, callback) {
        if (isDesktop()) {
            // Используем Wails runtime
            try {
                // Динамический импорт runtime не нужен — он уже доступен
                // через window.runtime (Wails v2 инжектит его глобально)
                if (window.runtime && window.runtime.EventsOn) {
                    window.runtime.EventsOn(event, callback);
                }
            } catch (e) {
                console.warn('[EventBridge] Wails EventsOn unavailable:', e);
            }
            return;
        }

        // Mobile: сохраняем подписку
        if (!this._listeners.has(event)) {
            this._listeners.set(event, []);
        }
        this._listeners.get(event).push(callback);

        // Запускаем SSE если ещё не стартовали
        this._ensureSSE();
    }

    /** Запускает Server-Sent Events соединение с Go сервером */
    _ensureSSE() {
        if (this._evtSource) return;

        try {
            this._evtSource = new EventSource(`${MOBILE_BASE_URL}/events`);

            this._evtSource.onmessage = (e) => {
                try {
                    const payload = JSON.parse(e.data);
                    const { event, data } = payload;
                    const cbs = this._listeners.get(event) || [];
                    cbs.forEach(cb => cb(data));
                } catch (err) {
                    console.warn('[EventBridge] SSE parse error:', err);
                }
            };

            this._evtSource.onerror = () => {
                console.warn('[EventBridge] SSE connection lost, reconnecting in 3s...');
                this._evtSource.close();
                this._evtSource = null;
                setTimeout(() => this._ensureSSE(), 3000);
            };
        } catch (err) {
            console.warn('[EventBridge] SSE unavailable, falling back to polling');
            this._startPolling();
        }
    }

    /** Fallback: polling для событий (если SSE недоступен) */
    _startPolling() {
        if (this._polling) return;
        this._polling = true;

        setInterval(async () => {
            try {
                const resp = await fetch(`${MOBILE_BASE_URL}/poll-events`);
                if (!resp.ok) return;
                const events = await resp.json();
                for (const { event, data } of events) {
                    const cbs = this._listeners.get(event) || [];
                    cbs.forEach(cb => cb(data));
                }
            } catch {
                // Тихо игнорируем — сервер ещё не стартовал
            }
        }, 2000);
    }
}

export const EventBridge = new MobileEventBridge();

// ─── Пример использования для CreateProfile: ───────────────────────────────
//
// import { Api } from '$lib/api_bridge.js';
//
// async function registerUser() {
//     try {
//         await Api.CreateProfile('Alice', '123456', '', '', '', true);
//         const profiles = await Api.ListProfiles();
//         console.log('Профили:', profiles);
//     } catch (err) {
//         console.error('Ошибка регистрации:', err);
//     }
// }
