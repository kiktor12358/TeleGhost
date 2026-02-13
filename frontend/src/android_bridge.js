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
// Wails Runtime Events Implementation
window.runtime = {
    EventsOn: (eventName, callback) => {
        if (!bridge.listeners[eventName]) {
            bridge.listeners[eventName] = [];
        }
        // Wrapper to maintain Wails behavior (maxCallbacks = -1 for infinite)
        bridge.listeners[eventName].push({ callback, maxCallbacks: -1 });
        return () => window.runtime.EventsOff(eventName, callback);
    },
    EventsOnMultiple: (eventName, callback, maxCallbacks) => {
        if (!bridge.listeners[eventName]) {
            bridge.listeners[eventName] = [];
        }
        bridge.listeners[eventName].push({ callback, maxCallbacks });
        return () => window.runtime.EventsOff(eventName, callback);
    },
    EventsOnce: (eventName, callback) => {
        return window.runtime.EventsOnMultiple(eventName, callback, 1);
    },
    EventsOff: (eventName, ...additionalEventNames) => {
        const events = [eventName, ...additionalEventNames];
        events.forEach(name => {
            delete bridge.listeners[name];
        });
    },
    EventsOffAll: () => {
        bridge.listeners = {};
    },
    EventsEmit: (eventName, data) => {
        // Broadcast locally to listeners (optimistic UI updates)
        if (bridge.listeners[eventName]) {
            bridge.listeners[eventName].forEach(listener => {
                listener.callback(data);
                if (listener.maxCallbacks > 0) {
                    listener.maxCallbacks--;
                }
            });
            // Cleanup exhausted listeners
            bridge.listeners[eventName] = bridge.listeners[eventName].filter(l => l.maxCallbacks !== 0);
        }
    }
};

// Override bridge listener logic to handle the new object structure
bridge.events.onmessage = (event) => {
    try {
        const msg = JSON.parse(event.data);
        // Expecting msg.event (string) and msg.data (payload)
        if (msg.event && bridge.listeners[msg.event]) {
            bridge.listeners[msg.event].forEach(listener => {
                listener.callback(msg.data);
                if (listener.maxCallbacks > 0) {
                    listener.maxCallbacks--;
                }
            });
            // Cleanup exhausted listeners
            bridge.listeners[msg.event] = bridge.listeners[msg.event].filter(l => l.maxCallbacks !== 0);
        }
    } catch (e) {
        console.error("Failed to parse SSE message:", e);
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
