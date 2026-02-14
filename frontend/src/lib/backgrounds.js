// Пак пресетов фонов — градиенты, изображения и компоненты.
// Типы: none, css (градиент/цвет), image (URL), component (Svelte)
const backgrounds = [
  { id: 'none', name: 'Нет', type: 'none', value: '' },
  { id: 'soft-gradient', name: 'Мягкий градиент', type: 'css', value: 'linear-gradient(180deg,#ffffff,#f3f4f6)' },
  { id: 'purple', name: 'Фиолетовый градиент', type: 'css', value: 'linear-gradient(135deg,#6c5ce7 0%, #a29bfe 100%)' },
  { id: 'sunset', name: 'Закат', type: 'css', value: 'linear-gradient(120deg,#ff9a9e 0%,#fecfef 100%)' },
  { id: 'ocean', name: 'Океан', type: 'css', value: 'linear-gradient(180deg,#a8edea 0%, #fed6e3 100%)' },
  { 
    id: 'darkveil-default', 
    name: 'DarkVeil — Cerebral', 
    type: 'component', 
    value: 'DarkVeil',
    config: { hueShift: 0, noiseIntensity: 0.05, scanlineIntensity: 0, speed: 0.5, scanlineFrequency: 0, warpAmount: 0 }
  },
  { 
    id: 'darkveil-glitch', 
    name: 'DarkVeil — Glitch', 
    type: 'component', 
    value: 'DarkVeil',
    config: { hueShift: 45, noiseIntensity: 0.15, scanlineIntensity: 0.3, speed: 0.3, scanlineFrequency: 50, warpAmount: 0.1 }
  },
  { 
    id: 'darkveil-neon', 
    name: 'DarkVeil — Neon', 
    type: 'component', 
    value: 'DarkVeil',
    config: { hueShift: 180, noiseIntensity: 0.1, scanlineIntensity: 0.2, speed: 0.8, scanlineFrequency: 30, warpAmount: 0.05 }
  },
  { id: 'photo-sample', name: 'Фото — пример', type: 'image', value: 'https://images.unsplash.com/photo-1503264116251-35a269479413?q=80&w=1200&auto=format&fit=crop&crop=entropy' }
];

export default backgrounds;

// Пример использования:
// import backgrounds from '../lib/backgrounds.js'
// backgrounds.forEach(b => console.log(b.id, b.name, b.type))
