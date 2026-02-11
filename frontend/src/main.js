import './style.css'
import './ui_styles.css'
import App from './App.svelte'

// Временный хак: если window.go нет, значит это браузер/Android WebView
if (!window.runtime) {
  // Импортируем скрипт как текст и исполняем (т.к. статический импорт перехватит Wails)
  // Или просто проверим:
  import('./android_bridge.js').then(() => {
    console.log("Loaded Android bridge");
    initApp();
  }).catch(e => {
    console.error("Failed to load bridge", e);
    initApp();
  });
} else {
  initApp();
}

function initApp() {
  const app = new App({
    target: document.getElementById('app')
  })
}

export default app
