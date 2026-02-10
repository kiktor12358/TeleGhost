<script>
    import { onMount } from 'svelte';
    import { fade, scale, fly, slide } from 'svelte/transition';
    import { Icons } from '../Icons.js';
    import { showToast } from '../stores.js';
    import { 
        Login, 
        CreateAccount, 
        CreateProfile, 
        ListProfiles, 
        UnlockProfile,
        GetFileBase64,
        CopyToClipboard
    } from '../../wailsjs/go/main/App.js';
    import { getInitials } from '../utils.js';

    export let logo;
    export let onLoginSuccess;

    let isLoading = false;
    let profilesLoaded = false;
    let authScreen = 'profiles'; // profiles | pin | seed | create
    let allProfiles = [];
    let selectedProfile = null;
    let pinInput = '';
    let seedPhrase = '';
    
    let newProfileName = '';
    let newProfilePin = '';
    let newProfileUsePin = true;
    let newProfileAvatarPath = '';
    let newProfileAvatarPreview = '';
    
    let profileAvatars = {};
    let showMnemonicModal = false;
    let newMnemonic = '';

    onMount(async () => {
        await loadProfiles();
    });

    async function loadProfiles() {
        try {
            const profiles = await ListProfiles();
            allProfiles = profiles || [];
            
            // Load avatars
            const newAvatars = {};
            for (const p of allProfiles) {
                if (p.avatar_path) {
                    try {
                        const base64 = await GetFileBase64(p.avatar_path);
                        if (base64) newAvatars[p.id] = base64;
                    } catch (e) {
                        console.error("Failed to load avatar for", p.id, e);
                    }
                }
            }
            profileAvatars = newAvatars;
            profilesLoaded = true;
            
            if (allProfiles.length === 0) {
                authScreen = 'profiles';
            }
        } catch (err) {
            showToast(err, 'error');
            profilesLoaded = true;
        }
    }



    function selectProfileForLogin(p) {
        selectedProfile = p;
        if (p.use_pin) {
            authScreen = 'pin';
            pinInput = '';
        } else {
            // No PIN, need seed? Or just login if decrypted?
            // Usually if no pin, we still need seed? 
            // The existing app seems to require seed if no pin.
            authScreen = 'seed';
            seedPhrase = '';
        }
    }

    async function handleUnlock() {
        if (!pinInput || !selectedProfile) return;
        isLoading = true;
        try {
            const mnemonic = await UnlockProfile(selectedProfile.id, pinInput);
            await handleLoginAction(mnemonic);
        } catch (err) {
            showToast('Неверный ПИН-код', 'error');
        } finally {
            isLoading = false;
        }
    }

    async function handleLogin() {
        if (!seedPhrase.trim()) return;
        isLoading = true;
        try {
            await handleLoginAction(seedPhrase);
        } catch (err) {
            showToast(err, 'error');
        } finally {
            isLoading = false;
        }
    }

    async function handleLoginAction(mnemonic) {
        await Login(mnemonic);
        await onLoginSuccess();
        showToast('Вход выполнен успешно');
    }

    function startCreateProfile() {
        authScreen = 'create';
        newProfileName = '';
        newProfilePin = '';
        newProfileUsePin = true;
        newProfileAvatarPath = '';
        newProfileAvatarPreview = '';
    }

    async function handleNewProfileAvatar(e) {
        const file = e.target.files[0];
        if (!file) return;
        newProfileAvatarPath = file.path; // Wails provides path 
        // For preview
        const reader = new FileReader();
        reader.onload = (e) => newProfileAvatarPreview = e.target.result;
        reader.readAsDataURL(file);
    }

    async function handleFinishCreateProfile() {
        if (!newProfileName) {
            showToast('Введите имя профиля', 'error');
            return;
        }
        if (newProfileUsePin && newProfilePin.length < 6) {
            showToast('ПИН-код должен быть не менее 6 цифр', 'error');
            return;
        }

        isLoading = true;
        try {
            const mnemonic = await CreateAccount();
            newMnemonic = mnemonic;
            
            await CreateProfile(
                newProfileName,
                newProfilePin,
                mnemonic,
                "", // userID will be derived from mnemonic in backend
                newProfileAvatarPath,
                newProfileUsePin
            );
            
            showMnemonicModal = true;
            isLoading = false;
        } catch (err) {
            showToast(err, 'error');
            isLoading = false;
        }
    }

    function cancelCreate() {
        authScreen = 'profiles';
        // Clear fields on cancel too
        newProfileName = '';
        newProfilePin = '';
        newProfileAvatarPath = '';
        newProfileAvatarPreview = '';
    }

    function confirmMnemonicSaved() {
        showMnemonicModal = false;
        // Clear fields here after saved
        newProfileName = '';
        newProfilePin = '';
        newProfileAvatarPath = '';
        newProfileAvatarPreview = '';
        handleLoginAction(newMnemonic);
    }
</script>

<div class="login-screen bg-animated" in:fade={{duration: 400}}>
  <div class="login-container glass-panel animate-fade-in" 
       in:scale={{duration: 500, start: 0.95}}
       style="max-width: {authScreen === 'profiles' ? '540px' : '440px'}; padding: 40px; border-radius: 28px;">
    
    <div class="login-logo" style="margin-bottom: 32px;">
      <img src="/icon.png" alt="TeleGhost" class="rounded-full shadow-lg" style="width: 80px; height: 80px; filter: drop-shadow(0 0 20px rgba(99, 102, 241, 0.4)); object-fit: cover; border: 2px solid rgba(255,255,255,0.1);" />
    </div>
    
    <h1 class="login-title" style="font-size: 32px; font-weight: 800; letter-spacing: -0.5px; margin-bottom: 8px; background: linear-gradient(to right, #fff, #a29bfe); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">TeleGhost</h1>

    {#if !profilesLoaded}
      <div style="padding: 60px 0; text-align: center;" out:fade>
        <div class="spinner" style="width: 40px; height: 40px; border-width: 4px; border-top-color: var(--accent); margin: 0 auto;"></div>
        <p style="margin-top: 24px; color: var(--text-secondary); font-size: 15px; font-weight: 500; letter-spacing: 0.5px;">Синхронизация профилей...</p>
      </div>
    {:else}
      <div in:fade={{duration: 300, delay: 100}}>
        {#if authScreen === 'profiles'}
          <p class="login-subtitle" style="color: var(--text-secondary); margin-bottom: 32px;">Выберите аккаунт для входа</p>
          
          <div class="profiles-grid">
            {#each allProfiles as p}
              <div class="profile-item animate-card" on:click={() => selectProfileForLogin(p)}>
                <div class="profile-avatar" style="background: rgba(255,255,255,0.05);">
                  {#if p.id && profileAvatars[p.id]}
                    <img src={"data:image/jpeg;base64," + profileAvatars[p.id]} alt="Avatar" />
                  {:else}
                    <div class="avatar-placeholder-mini" style="background: var(--accent);">{getInitials(p.display_name)}</div>
                  {/if}
                </div>
                <div class="profile-name">{p.display_name || 'Неизвестный'}</div>
                {#if !p.id}
                    <div class="legacy-badge">Legacy</div>
                {/if}
              </div>
            {/each}
            
            <div class="profile-item add-profile" on:click={startCreateProfile}>
              <div class="plus-icon">+</div>
              <div class="add-text">Создать</div>
            </div>
          </div>
          
          <div class="divider-text">
            <div class="line"></div>
            <span>или</span>
            <div class="line"></div>
          </div>
          
          <button class="btn-glass full-width" on:click={() => authScreen = 'seed'}>
            Войти по seed-фразе
          </button>

        {:else if authScreen === 'pin'}
          <div in:fly={{y: 20, duration: 400}}>
            <div class="profile-avatar-large">
                {#if selectedProfile && profileAvatars[selectedProfile.id]}
                    <img src={"data:image/jpeg;base64," + profileAvatars[selectedProfile.id]} alt="Avatar" />
                {:else}
                    {getInitials(selectedProfile?.display_name)}
                {/if}
            </div>
            <p class="login-subtitle">Введите ПИН для <b>{selectedProfile?.display_name}</b></p>
            <div class="pin-entry-box">
              <input 
                type="password" 
                class="input-premium" 
                placeholder="••••" 
                bind:value={pinInput} 
                on:keydown={(e) => e.key === 'Enter' && handleUnlock()}
                autoFocus 
              />
              <button class="btn-primary-premium full-width" on:click={handleUnlock} disabled={isLoading || pinInput.length < 1}>
                {#if isLoading}<span class="spinner"></span>{:else}Разблокировать{/if}
              </button>
              <button class="btn-link back-btn" on:click={() => authScreen = 'profiles'}>
                ← Назад к списку
              </button>
            </div>
          </div>

        {:else if authScreen === 'create'}
          <div in:fly={{y: 20, duration: 400}}>
            <p class="login-subtitle">Новый профиль</p>
            
            <div class="create-avatar-upload">
                <div class="avatar-preview-box" on:click={() => document.getElementById('newProfileAvatarInput').click()}>
                    {#if newProfileAvatarPreview}
                        <img src={newProfileAvatarPreview} alt="Preview" />
                    {:else}
                        <div class="icon-svg">{@html Icons.Camera}</div>
                    {/if}
                    <div class="hover-overlay">
                        <span>Изменить</span>
                    </div>
                </div>
                <input type="file" id="newProfileAvatarInput" accept="image/*" style="display: none;" on:change={handleNewProfileAvatar} />
            </div>

            <div class="create-form">
              <input type="text" class="input-premium-small" placeholder="Имя профиля" bind:value={newProfileName} maxLength="20" />
              
              <label class="use-pin-label">
                  <input type="checkbox" bind:checked={newProfileUsePin} />
                  <span>Использовать ПИН-код для входа</span>
              </label>

              {#if newProfileUsePin}
                <div in:slide={{duration: 200}}>
                    <input type="password" class="input-premium-small full-width" placeholder="ПИН-код (минимум 6 цифр)" bind:value={newProfilePin} />
                    <p class="hint">ПИН-код шифрует ваш ключ локально. Без него вход возможен только по seed-фразе.</p>
                </div>
              {:else}
                <p class="warning-hint">Внимание: При каждом входе вам придется вводить 12 слов seed-фразы вручную.</p>
              {/if}
              
              <button class="btn-primary-premium full-width" on:click={handleFinishCreateProfile} disabled={isLoading}>
                {#if isLoading}<span class="spinner"></span>{:else}Создать профиль{/if}
              </button>
              <button class="btn-glass full-width cancel-btn" on:click={cancelCreate} style="margin-top: 8px;">Отмена</button>
            </div>
          </div>

        {:else if authScreen === 'seed'}
          <div in:fly={{y: 20, duration: 400}}>
            <p class="login-subtitle">Вход по фразе</p>
            <div class="seed-form">
              <textarea
                class="seed-input-premium"
                placeholder="12 слов через пробел..."
                bind:value={seedPhrase}
                rows="3"
              ></textarea>
              
              <button class="btn-primary-premium full-width" on:click={handleLogin} disabled={isLoading} style="margin-bottom: 24px;">
                {#if isLoading}<span class="spinner"></span>{:else}Войти в чат{/if}
              </button>
              
              <button class="btn-text full-width" on:click={() => authScreen = 'profiles'}>← Назад к профилям</button>
            </div>
          </div>
        {/if}
      </div>
    {/if}

    <p class="login-footer">🔒 ВСЕ ДАННЫЕ ЗАШИФРОВАНЫ И ХРАНЯТСЯ ЛОКАЛЬНО</p>
  </div>
</div>

{#if showMnemonicModal}
<div class="modal-backdrop animate-fade-in">
  <div class="modal-content animate-slide-down">
    <div class="modal-header">
      <h2>🔐 Ваш секретный ключ</h2>
    </div>
    <div class="modal-body">
      <p class="warning-text"><span class="icon-svg-sm">{@html Icons.AlertTriangle}</span> Сохраните эти 12 слов. Без них восстановить доступ невозможно!</p>
      
      <div class="mnemonic-grid">
        {#each newMnemonic.split(' ') as word, i}
          <div class="mnemonic-word">
            <span class="word-index">{i+1}</span>
            <span class="word-text">{word}</span>
          </div>
        {/each}
      </div>

      <div class="mnemonic-actions">
         <button class="btn-glass clickable-btn" on:click={() => { CopyToClipboard(newMnemonic); showToast('Скопировано в буфер обмена', 'success'); }} style="padding: 12px 24px; border-radius: 16px;">
           📋 Скопировать всё
         </button>
      </div>
    </div>
    <div class="modal-footer" style="position: relative; z-index: 10001;">
      <button class="btn-primary-premium full-width accent-btn clickable-btn" on:click|preventDefault|stopPropagation={confirmMnemonicSaved} style="height: 52px; font-size: 16px; border-radius: 16px;">
        Я сохранил(а) seed-фразу
      </button>
    </div>
  </div>
</div>
{/if}

<style>
  .login-screen {
    width: 100vw;
    height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    position: relative;
  }

  .login-container {
    width: 100%;
    margin: 20px;
    text-align: center;
    z-index: 10;
    max-height: 85vh;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    scrollbar-width: none;
  }
  .login-container::-webkit-scrollbar { display: none; }

  .profiles-grid {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 20px;
    margin-bottom: 32px;
  }

  .profile-item {
    background: rgba(255,255,255,0.05);
    padding: 24px 16px;
    border-radius: 24px;
    cursor: pointer;
    border: 1px solid rgba(255,255,255,0.05);
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    position: relative;
    overflow: hidden;
  }
  .profile-item:hover { transform: translateY(-5px); background: rgba(255,255,255,0.1); border-color: rgba(255,255,255,0.2); }

  .profile-avatar {
    width: 64px;
    height: 64px;
    margin: 0 auto 16px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 28px;
    color: white;
    box-shadow: 0 10px 20px rgba(0,0,0,0.2);
    overflow: hidden;
  }
  .profile-avatar img { width: 100%; height: 100%; object-fit: cover; }

  .profile-name { font-weight: 600; font-size: 15px; text-align: center; color: #fff; }
  .legacy-badge { font-size: 10px; color: rgba(255,255,255,0.5); text-align: center; }

  .add-profile {
    background: rgba(99, 102, 241, 0.1) !important;
    border: 2px dashed rgba(99, 102, 241, 0.3) !important;
    display: flex; flex-direction: column; align-items: center; justify-content: center;
  }
  .plus-icon { font-size: 32px; color: var(--accent, #6366f1); margin-bottom: 12px; font-weight: 300; }
  .add-text { font-size: 13px; font-weight: 600; color: var(--accent, #6366f1); text-transform: uppercase; letter-spacing: 1px; }

  .divider-text {
    display: flex; align-items: center; margin-bottom: 24px; color: rgba(255,255,255,0.2); font-size: 12px; font-weight: 700; text-transform: uppercase; letter-spacing: 2px;
  }
  .divider-text .line { flex: 1; height: 1px; background: rgba(255,255,255,0.1); }
  .divider-text span { padding: 0 16px; }

  .profile-avatar-large {
    width: 80px; height: 80px; margin: 0 auto 20px; border-radius: 50%; overflow: hidden; box-shadow: 0 5px 15px rgba(0,0,0,0.3); background: var(--bg-secondary, #1e1e2e); display: flex; align-items: center; justify-content: center; font-size: 32px; color: white;
  }
  .profile-avatar-large img { width: 100%; height: 100%; object-fit: cover; }

  .input-premium {
    text-align: center; font-size: 36px; letter-spacing: 12px; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; width: 100%; padding: 20px; border-radius: 18px; margin-bottom: 24px; outline: none;
  }
  .btn-primary-premium {
    padding: 18px; border-radius: 18px; background: var(--accent, #6366f1); color: white; border: none; font-weight: 700; font-size: 16px; cursor: pointer; transition: all 0.3s; margin-bottom: 16px;
  }

  .create-avatar-upload { display: flex; justify-content: center; margin-bottom: 20px; }
  .avatar-preview-box {
    width: 90px; height: 90px; border-radius: 50%; background: rgba(255,255,255,0.1); display: flex; align-items: center; justify-content: center; position: relative; cursor: pointer; overflow: hidden; border: 2px dashed rgba(255,255,255,0.2);
  }
  .avatar-preview-box img { width: 100%; height: 100%; object-fit: cover; }
  .avatar-preview-box .hover-overlay {
    position: absolute; inset: 0; background: rgba(0,0,0,0.3); display: flex; align-items: center; justify-content: center; opacity: 0; transition: opacity 0.2s;
  }
  .avatar-preview-box:hover .hover-overlay { opacity: 1; }

  .create-form { display: flex; flex-direction: column; gap: 16px; }
  .input-premium-small {
    background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 14px 18px; border-radius: 14px; outline: none;
  }
  .use-pin-label {
    display: flex; align-items: center; gap: 10px; cursor: pointer; background: rgba(0,0,0,0.2); padding: 12px; border-radius: 12px;
  }
  .use-pin-label input { width: 18px; height: 18px; accent-color: var(--accent, #6366f1); }
  .hint { font-size: 11px; color: var(--text-secondary, #a0a0ba); text-align: left; padding: 4px; margin-top: 4px; }
  .warning-hint { font-size: 11px; color: #ffcc00; text-align: left; padding: 4px; }

  .seed-input-premium {
    background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 18px; border-radius: 18px; outline: none; resize: none; font-family: monospace; font-size: 14px; line-height: 1.6; width: 100%;
  }

  .login-footer { margin-top: 40px; font-size: 11px; color: rgba(255,255,255,0.3); font-weight: 500; letter-spacing: 0.5px; }

  .btn-glass {
    border: 1px solid rgba(255,255,255,0.1); background: rgba(255,255,255,0.03); color: #fff; padding: 14px; border-radius: 16px; font-weight: 600; cursor: pointer; transition: all 0.2s;
  }
  .accent-text { color: var(--accent, #6366f1); border-color: rgba(99, 102, 241, 0.3); }

  .full-width { width: 100%; }

  .modal-backdrop {
    position: fixed; top: 0; left: 0; width: 100%; height: 100%;
    background: rgba(0,0,0,0.85); backdrop-filter: blur(8px);
    display: flex; align-items: center; justify-content: center; z-index: 10000;
  }
  .modal-content {
    background: #1e1e2e; border-radius: 28px; padding: 32px;
    width: 90%; max-width: 480px; border: 1px solid rgba(255,255,255,0.1);
    box-shadow: 0 25px 50px rgba(0,0,0,0.6);
  }
  .modal-header h2 { font-size: 24px; color: #fff; margin-bottom: 16px; text-align: center; }
  .warning-text { 
    background: rgba(255, 107, 107, 0.1); color: #ff6b6b; padding: 16px; 
    border-radius: 12px; font-size: 13px; line-height: 1.5; margin-bottom: 24px;
    display: flex; align-items: flex-start; gap: 10px;
  }
  .mnemonic-grid { 
    display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin-bottom: 24px;
  }
  .mnemonic-word {
    background: rgba(255,255,255,0.05); padding: 10px; border-radius: 12px;
    display: flex; align-items: center; gap: 8px; border: 1px solid rgba(255,255,255,0.05);
  }
  .word-index { font-size: 10px; color: rgba(255,255,255,0.3); font-weight: 700; width: 14px; }
  .word-text { font-size: 13px; color: #fff; font-weight: 500; }
  .mnemonic-actions { text-align: center; margin-bottom: 24px; }
  .btn-text { background: none; border: none; color: var(--accent); cursor: pointer; font-size: 14px; font-weight: 600; transition: all 0.2s; }
  .btn-text:hover { filter: brightness(1.2); }

  .btn-link { background: none; border: none; color: var(--text-secondary); cursor: pointer; font-size: 14px; margin-top: 10px; transition: color 0.2s; }
  .btn-link:hover { color: #fff; }
  .back-btn { 
      display: block;
      margin: 16px auto 0;
      opacity: 0.7;
  }
  .cancel-btn {
      background: transparent !important;
      border: 1px solid rgba(255,255,255,0.1) !important;
      color: rgba(255,255,255,0.5) !important;
  }
  .cancel-btn:hover {
      border-color: rgba(255,255,255,0.3) !important;
      color: #fff !important;
  }
  .accent-btn { background: #6366f1 !important; }

  .avatar-placeholder-mini {
      width: 100%; height: 100%; display: flex; align-items: center; justify-content: center;
      color: white; font-size: 18px; font-weight: 700;
  }
  .avatar-placeholder-large {
      width: 100%; height: 100%; display: flex; align-items: center; justify-content: center;
      color: white; font-size: 32px; font-weight: 700;
  }
  .clickable-btn { cursor: pointer !important; position: relative; z-index: 10002; }
  .clickable-btn:hover { filter: brightness(1.1); transform: translateY(-1px); }
  .clickable-btn:active { transform: translateY(0); }
</style>
