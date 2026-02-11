export class WailsBridge {
    constructor() {
        this.events = new EventSource("/api/events");
        this.apiBase = "/api/";
        this.listeners = {};

        this.events.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                if (msg.event && this.listeners[msg.event]) {
                    this.listeners[msg.event].forEach(cb => cb(msg.data));
                }
            } catch (e) {
                console.error("Failed to parse SSE message:", e);
            }
        };
    }

    async call(method, ...args) {
        const response = await fetch(this.apiBase + method, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ args: args })
        });

        if (!response.ok) {
            throw new Error(`RPC Error: ${response.statusText}`);
        }

        const data = await response.json();
        if (data.error) {
            throw new Error(data.error);
        }
        return data.result;
    }
}

const bridge = new WailsBridge();

// Функция-прокси для имитации window.go.main.App
function createProxy(methodName) {
    return (...args) => bridge.call(methodName, ...args);
}

// Инициализация глобального объекта runtime для Wails Events
window.runtime = {
    EventsOn: (eventName, callback) => {
        if (!bridge.listeners[eventName]) {
            bridge.listeners[eventName] = [];
        }
        bridge.listeners[eventName].push(callback);
    },
    EventsOff: (eventName) => {
        delete bridge.listeners[eventName];
    },
    EventsEmit: (eventName, data) => {
        // Локальная эмуляция или отправка на сервер если нужно
        console.log("EventsEmit not fully implemented in bridge", eventName, data);
    }
};

// Инициализация глобального объекта window.go
window.go = {
    main: {
        App: new Proxy({}, {
            get: function (target, prop) {
                return createProxy(prop);
            }
        })
    }
};

console.log("Android bridge initialized");
