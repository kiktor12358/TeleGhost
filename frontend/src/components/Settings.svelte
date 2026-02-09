<script>
    import { Icons } from '../Icons.js';
    import { getInitials, getStatusColor, getStatusText } from '../utils.js';

    export let profileNickname;
    export let profileBio;
    export let profileAvatar;
    export let routerSettings;
    export let settingsCategories;
    export let activeSettingsTab;
    export let settingsView;
    export let selectedProfile;
    export let networkStatus;
    export let myDestination;

    export let onSaveProfile;
    export let onSaveRouterSettings;
    export let onAvatarChange;
    export let onLogout;
    export let onTogglePinUsage;
    export let onChangePin;
    export let onBackToMenu;
    export let onOpenCategory;
    export let onClose;

    let avatarFileInput;
</script>

<div class="settings-panel animate-fade-in">
    {#if settingsView === 'menu'}
        <div class="settings-view-menu">
            <div class="settings-header">
                <h2>Настройки</h2>
                <button class="btn-icon" on:click={onClose}>
                    <div class="icon-svg">{@html Icons.X}</div>
                </button>
            </div>

            <div class="settings-menu">
                {#each settingsCategories as cat}
                    <div class="settings-menu-item" on:click={() => onOpenCategory(cat.id)}>
                        <span class="icon-svg">{@html cat.icon}</span>
                        <span class="name">{cat.name}</span>
                        <span class="arrow">{@html Icons.ChevronRight}</span>
                    </div>
                {/each}
            </div>
        </div>
    {:else}
        <div class="settings-view-details">
            <div class="settings-header">
                <button class="btn-icon" on:click={onBackToMenu}>
                    <div class="icon-svg">{@html Icons.ArrowLeft}</div>
                </button>
                <h2>{settingsCategories.find(c => c.id === activeSettingsTab)?.name}</h2>
            </div>

            <div class="settings-content-area">
                {#if activeSettingsTab === 'profile'}
                    <div class="settings-section">
                        <div class="profile-avatar-large">
                            {#if profileAvatar}
                                <img src={profileAvatar} alt="Avatar" />
                            {:else}
                                <div class="avatar-placeholder">{getInitials(profileNickname)}</div>
                            {/if}
                            <button class="avatar-edit-btn" on:click={() => avatarFileInput.click()}>
                                <div class="icon-svg-sm">{@html Icons.Camera}</div>
                            </button>
                            <input type="file" bind:this={avatarFileInput} on:change={onAvatarChange} accept="image/*" style="display: none;" />
                        </div>
                        
                        <div class="profile-fields">
                            <label class="form-label">Никнейм
                                <input type="text" bind:value={profileNickname} class="input-field" placeholder="Ваш никнейм" />
                            </label>
                            <label class="form-label">О себе
                                <textarea bind:value={profileBio} class="input-field" rows="3" placeholder="Расскажите о себе..."></textarea>
                            </label>
                            <button class="btn-primary" on:click={onSaveProfile}>💾 Сохранить изменения</button>
                            <button class="btn-secondary logout-btn" on:click={onLogout}>🚪 Выйти из аккаунта</button>
                        </div>
                    </div>
                {:else if activeSettingsTab === 'privacy'}
                    <div class="settings-section">
                         <div class="info-box danger">
                            <h4>🔐 Секретный ключ (Seed phrase)</h4>
                            <p>Ваш ключ хранится только на этом устройстве. Если вы потеряете его, доступ к аккаунту будет утерян навсегда.</p>
                            <button class="btn-secondary" on:click={() => {/* Show seed modal */}}>Показать ключ</button>
                         </div>

                         {#if selectedProfile}
                            <div class="setting-item-box">
                                <div class="flex-row">
                                    <div>
                                        <h4>Быстрый вход по ПИН-коду</h4>
                                        <p>{selectedProfile.use_pin ? 'Включено. Ключ зашифрован локально.' : 'Выключено. Требуется ввод seed-фразы.'}</p>
                                    </div>
                                    <label class="switch">
                                        <input type="checkbox" checked={selectedProfile.use_pin} on:change={onTogglePinUsage}>
                                        <span class="slider round"></span>
                                    </label>
                                </div>
                                {#if selectedProfile.use_pin}
                                    <button class="btn-secondary full-width" on:click={onChangePin}>Сменить ПИН-код</button>
                                {/if}
                            </div>
                         {/if}
                    </div>
                {:else if activeSettingsTab === 'network'}
                    <div class="settings-section">
                        <label class="form-label">Ваш I2P адрес (Destination)</label>
                        <div class="destination-box">
                            <code class="destination-code">{myDestination ? myDestination.slice(0, 50) + '...' : 'Загрузка...'}</code>
                            <button class="btn-icon-copy" on:click={() => navigator.clipboard.writeText(myDestination)}>📋</button>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Статус сети:</span>
                            <span class="info-value" style="color: {getStatusColor(networkStatus)}">{getStatusText(networkStatus)}</span>
                        </div>

                        <h4 class="section-title">Настройки роутера</h4>
                        <div class="settings-item-group">
                            <div class="setting-item">
                                <label class="form-label">Режим анонимности (длина туннелей)</label>
                                <select bind:value={routerSettings.tunnelLength} class="input-field">
                                    <option value={1}>Fast (1 хоп)</option>
                                    <option value={2}>Normal (2 хопа)</option>
                                    <option value={4}>Invisible (4 хопа)</option>
                                </select>
                            </div>
                            <div class="setting-item flex-row bg-box">
                                <div>
                                    <span class="label">Логирование в файл</span>
                                    <p class="hint">Записывать логи роутера в i2pd.log</p>
                                </div>
                                <input type="checkbox" bind:checked={routerSettings.logToFile} />
                            </div>
                            <button class="btn-primary full-width" on:click={onSaveRouterSettings}>💾 Сохранить и применить</button>
                        </div>
                    </div>
                {:else if activeSettingsTab === 'about'}
                    <div class="settings-section">
                        <h3 class="title">О программе</h3>
                        <div class="info-grid">
                            <div class="info-row"><span class="label">Версия</span><span class="value">1.0.2-beta</span></div>
                            <div class="info-row"><span class="label">Разработчик</span><span class="value">TeleGhost Team</span></div>
                            <div class="info-row"><span class="label">Лицензия</span><span class="value">MIT / Open Source</span></div>
                        </div>
                    </div>
                {/if}
            </div>
        </div>
    {/if}
</div>

<style>
    .settings-panel { flex: 1; display: flex; flex-direction: column; height: 100%; background: var(--bg-primary); }
    .settings-header { padding: 24px 40px; display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid var(--border); }
    .settings-header h2 { font-size: 24px; font-weight: 700; margin: 0; color: white; }
    
    .settings-view-menu { width: 100%; max-width: 600px; margin: 0 auto; flex: 1; display: flex; flex-direction: column; }
    .settings-menu { padding: 20px; }
    .settings-menu-item {
        padding: 16px; margin-bottom: 8px; background: var(--bg-secondary); border-radius: 18px; display: flex; align-items: center; gap: 16px; cursor: pointer; transition: transform 0.2s, background 0.2s;
    }
    .settings-menu-item:hover { transform: translateX(5px); background: rgba(255,255,255,0.05); }
    .settings-menu-item .name { flex: 1; font-weight: 500; }
    .settings-menu-item .arrow { opacity: 0.3; }

    .settings-view-details { width: 100%; height: 100%; display: flex; flex-direction: column; }
    .settings-content-area { flex: 1; padding: 40px; overflow-y: auto; max-width: 800px; margin: 0 auto; width: 100%; }

    .profile-avatar-large { width: 120px; height: 120px; position: relative; margin: 0 auto 32px; }
    .profile-avatar-large img { width: 100%; height: 100%; object-fit: cover; border-radius: 50%; box-shadow: 0 5px 15px rgba(0,0,0,0.3); }
    .avatar-placeholder { width: 100%; height: 100%; background: linear-gradient(135deg, #6c5ce7, #a29bfe); border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 48px; color: white; }
    .avatar-edit-btn { position: absolute; bottom: 0; right: 0; background: var(--accent); border: 4px solid var(--bg-primary); border-radius: 50%; width: 40px; height: 40px; display: flex; align-items: center; justify-content: center; cursor: pointer; color: white; }

    .profile-fields { max-width: 500px; margin: 0 auto; display: flex; flex-direction: column; gap: 24px; }
    .input-field { width: 100%; padding: 12px; background: var(--bg-input); border: 1px solid var(--border); border-radius: 12px; color: white; outline: none; }
    
    .btn-primary { background: var(--accent); color: white; padding: 14px; border: none; border-radius: 12px; font-weight: 600; cursor: pointer; }
    .btn-secondary { background: transparent; border: 1px solid var(--accent); color: var(--accent); padding: 12px; border-radius: 12px; cursor: pointer; }
    .logout-btn { border-color: #f44336; color: #f44336; margin-top: 12px; }

    .info-box { padding: 24px; border-radius: 12px; margin-bottom: 24px; }
    .info-box.danger { background: rgba(255, 100, 100, 0.1); border: 1px solid rgba(255, 100, 100, 0.3); }
    .info-box h4 { margin: 0 0 12px; color: #ff6b6b; }

    .setting-item-box { background: var(--bg-secondary); padding: 20px; border-radius: 12px; }
    .flex-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
    .bg-box { background: var(--bg-secondary); padding: 16px; border-radius: 12px; }

    .destination-box { display: flex; align-items: center; gap: 10px; background: var(--bg-input); padding: 12px; border-radius: 12px; margin-top: 8px; }
    .destination-code { flex: 1; font-family: monospace; font-size: 12px; opacity: 0.7; overflow: hidden; text-overflow: ellipsis; }

    .full-width { width: 100%; }
    .section-title { margin: 32px 0 16px; border-top: 1px solid var(--border); padding-top: 24px; }

    .icon-svg { width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; }
    .icon-svg :global(svg) { width: 100%; height: 100%; }
    .icon-svg-sm { width: 18px; height: 18px; display: flex; align-items: center; justify-content: center; }
    .icon-svg-sm :global(svg) { width: 100%; height: 100%; }

    /* Switch styles */
    .switch { position: relative; display: inline-block; width: 50px; height: 24px; flex-shrink: 0; }
    .switch input { opacity: 0; width: 0; height: 0; }
    .slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: #333; transition: .4s; }
    .slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .4s; }
    input:checked + .slider { background-color: var(--accent); }
    input:checked + .slider:before { transform: translateX(26px); }
    .slider.round { border-radius: 34px; }
    .slider.round:before { border-radius: 50%; }
</style>
