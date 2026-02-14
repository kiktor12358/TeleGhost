<script>
  import { onMount } from 'svelte';
  import presets from '../lib/backgrounds.js';
  import DarkVeil from './DarkVeil.svelte';

  const STORAGE_KEY = 'app_backgrounds_v1';
  export let current = null;

  // Начинаем с пресетов; пользовательские будут добавляться сверху
  let backgrounds = [...presets];

  function load() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw) {
        const user = JSON.parse(raw);
        if (Array.isArray(user)) backgrounds = [...user, ...presets];
      }
    } catch (e) {}
  }

  function saveUserList() {
    try {
      const user = backgrounds.filter(b => b.id && b.id.startsWith('user-'));
      localStorage.setItem(STORAGE_KEY, JSON.stringify(user));
    } catch (e) {}
  }

  function apply(bg) {
    current = bg;
    if (!bg || bg.type === 'none') {
      document.documentElement.style.removeProperty('--app-bg');
    } else if (bg.type === 'image') {
      document.documentElement.style.setProperty('--app-bg', `url("${bg.value}")`);
    } else if (bg.type === 'css') {
      document.documentElement.style.setProperty('--app-bg', bg.value);
    } else if (bg.type === 'component') {
      // Компоненты рендерятся через слот выше
      document.documentElement.style.removeProperty('--app-bg');
    }
  }

  function onFile(e) {
    const f = e.target.files && e.target.files[0];
    if (!f) return;
    const fr = new FileReader();
    fr.onload = () => {
      const id = 'user-' + Date.now();
      const bg = { id, name: f.name, type: 'image', value: fr.result };
      backgrounds = [bg, ...backgrounds];
      saveUserList();
      apply(bg);
    };
    fr.readAsDataURL(f);
  }

  function removeBg(id) {
    backgrounds = backgrounds.filter(b => b.id !== id);
    saveUserList();
    if (current && current.id === id) apply(null);
  }

  onMount(load);
</script>

<style>
  .bg-list { display:flex; gap:8px; flex-wrap:wrap; }
  .bg-item { width:72px; height:48px; border-radius:8px; overflow:hidden; cursor:pointer; border:1px solid rgba(0,0,0,.06); display:flex; align-items:center; justify-content:center; font-size:11px; color:#111; position:relative; background:#fff; outline:none; }
  .bg-item:hover { background: #f5f5f5; }
  .bg-item:focus { outline: 2px solid #6c5ce7; outline-offset: 2px; }
  .bg-item.selected { outline: 2px solid #6c5ce7; }
  .bg-preview { width:100%; height:100%; background-size:cover; background-position:center; }
  .component-preview { width:100%; height:100%; background: linear-gradient(135deg,#1a1a2e 0%, #16213e 100%); display:flex; align-items:center; justify-content:center; color:#fff; }
  .remove { position:absolute; top:4px; right:4px; background:rgba(0,0,0,.4); color:white; border-radius:50%; width:18px; height:18px; font-size:12px; display:flex; align-items:center; justify-content:center; border:none; cursor:pointer; padding:0; line-height:1; }
  .remove:hover { background:rgba(0,0,0,.6); }
  .remove:focus { outline: 1px solid #fff; }
  .controls { display:flex; gap:8px; align-items:center; margin-bottom:8px; }
  .btn { padding:6px 10px; border-radius:8px; border:1px solid rgba(0,0,0,.08); background:#fff; cursor:pointer; }
  .btn:hover { background: #f5f5f5; }
  .btn:focus { outline: 2px solid #6c5ce7; }
</style>

<div>
  <div class="controls">
    <label class="btn">Добавить изображение
      <input type="file" accept="image/*" on:change={onFile} style="display:none" />
    </label>
    <button class="btn" on:click={() => apply(null)}>Убрать фон</button>
  </div>

  <div class="bg-list">
    {#each backgrounds as b (b.id)}
      <div class="bg-item {current && current.id === b.id ? 'selected' : ''}" on:click={() => apply(b)} on:keydown={(e) => e.key === 'Enter' && apply(b)} role="button" tabindex="0" title="{b.name}">
        {#if b.type === 'component'}
          <div class="component-preview">◆</div>
        {:else}
          <div class="bg-preview" style="background: {b.type === 'image' ? `url('${b.value}')` : (b.type === 'css' ? b.value : 'transparent')}"></div>
        {/if}
        {#if b.id && b.id.startsWith('user-')}
          <button class="remove" on:click|stopPropagation={() => removeBg(b.id)} title="Удалить" type="button">×</button>
        {/if}
      </div>
    {/each}
  </div>
</div>

{#if current && current.type === 'component'}
  <div style="position: fixed; inset: 0; z-index: -1; pointer-events: none;">
    {#if current.value === 'DarkVeil'}
      <DarkVeil 
        hueShift={current.config?.hueShift || 0}
        noiseIntensity={current.config?.noiseIntensity || 0}
        scanlineIntensity={current.config?.scanlineIntensity || 0}
        speed={current.config?.speed || 0.5}
        scanlineFrequency={current.config?.scanlineFrequency || 0}
        warpAmount={current.config?.warpAmount || 0}
      />
    {/if}
  </div>
{/if}
