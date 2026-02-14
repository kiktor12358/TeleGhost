import { writable } from 'svelte/store';
import { Api } from '../lib/api_bridge.js';

/**
 * Хранилище тем оформления
 * Синхронизирует параметры визуального оформления с бэком
 */

const defaultTheme = {
  darkVeil: {
    hueShift: 0,
    noiseIntensity: 0,
    scanlineIntensity: 0,
    speed: 0.5,
    scanlineFrequency: 0,
    warpAmount: 0,
    resolutionScale: 1,
    enabled: true
  },
  colors: {
    primary: '#6366f1',
    secondary: '#8b5cf6',
    accent: '#ec4899',
    background: '#0f172a',
    surface: '#1e293b'
  },
  animations: {
    enabled: true,
    duration: 300
  }
};

export const themeStore = writable(defaultTheme);

/**
 * Загружает тему с бэка
 */
export async function loadTheme() {
  try {
    const theme = await Api.GetSettings?.();
    if (theme?.theme) {
      themeStore.set({ ...defaultTheme, ...theme.theme });
    }
  } catch (error) {
    console.error('Ошибка загрузки темы:', error);
  }
}

/**
 * Сохраняет тему на бэке
 */
export async function saveTheme(theme) {
  try {
    await Api.UpdateSettings?.({ theme });
    themeStore.set(theme);
  } catch (error) {
    console.error('Ошибка сохранения темы:', error);
  }
}

/**
 * Обновляет параметры DarkVeil
 */
export function updateDarkVeil(params) {
  themeStore.update(theme => ({
    ...theme,
    darkVeil: { ...theme.darkVeil, ...params }
  }));
}

/**
 * Обновляет цвета темы
 */
export function updateColors(colors) {
  themeStore.update(theme => ({
    ...theme,
    colors: { ...theme.colors, ...colors }
  }));
}
