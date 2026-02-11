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
    export let aboutInfo;

    export let onSaveProfile;
    export let onSaveRouterSettings;
    export let onAvatarChange;
    export let onLogout;
    export let onTogglePinUsage;
    export let onChangePin;
    export let onBackToMenu;
    export let onOpenCategory;
    export let onClose;
    export let onShowSeed;
    export let onCheckUpdates;
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
                <h2>{settingsCategories.find(c => c.id === activeSettingsTab)?.name}</h2>
                <button class="btn-icon" on:click={onBackToMenu}>
                    <div class="icon-svg">{@html Icons.ArrowLeft}</div>
                </button>
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
                            <button class="avatar-edit-btn" on:click={onAvatarChange}>
                                <div class="icon-svg-sm">{@html Icons.Camera}</div>
                            </button>
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
                                    <button class="btn-secondary clickable-accent show-key-btn" on:click={onShowSeed}>Показать ключ</button>
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
                        <details class="destination-details">
                            <summary class="form-label">Ваш I2P адрес (Destination) <span class="hint-inline">(нажмите, чтобы развернуть)</span></summary>
                            <div class="destination-box">
                                <code class="destination-code">{myDestination || 'Загрузка...'}</code>
                                <button class="btn-icon-copy" on:click={() => {
                                    navigator.clipboard.writeText(myDestination);
                                }}>📋</button>
                            </div>
                        </details>
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
                            <button class="btn-primary full-width" on:click={onSaveRouterSettings} style="margin-top: 10px;">💾 Сохранить и применить</button>
                        </div>
                    </div>
                {:else if activeSettingsTab === 'about'}
                    <div class="settings-section animate-fade-in">
                        <div class="about-branding">
                            <div class="about-logo animate-float">
                                <div class="icon-svg-lg">{@html Icons.Ghost}</div>
                            </div>
                            <h2 class="about-title">TeleGhost</h2>
                            <p class="about-version">Version {aboutInfo.app_version || '1.0.2-beta'}</p>
                        </div>

                        <div class="info-list">
                            <div class="info-item-fancy">
                                <div class="info-icon">{@html Icons.User}</div>
                                <div class="info-content">
                                    <span class="info-label-fancy">Разработчик</span>
                                    <span class="info-value-fancy">{aboutInfo.author || 'TeleGhost Team'}</span>
                                </div>
                            </div>

                            <div class="info-item-fancy">
                                <div class="info-icon">{@html Icons.File}</div>
                                <div class="info-content">
                                    <span class="info-label-fancy">Лицензия</span>
                                    <span class="info-value-fancy">{aboutInfo.license || 'MIT / Open Source'}</span>
                                </div>
                            </div>

                            <div class="info-item-fancy">
                                <div class="info-icon">{@html Icons.Globe}</div>
                                <div class="info-content">
                                    <span class="info-label-fancy">I2P Версия</span>
                                    <span class="info-value-fancy">{aboutInfo.i2p_version || '2.58.0'}</span>
                                </div>
                            </div>

                            <div class="info-item-fancy">
                                <div class="info-icon">{@html Icons.Folder}</div>
                                <div class="info-content">
                                    <span class="info-label-fancy">Путь I2P</span>
                                    <span class="info-value-fancy" style="font-family: monospace; font-size: 11px; word-break: break-all;">{aboutInfo.i2p_path || '~/.teleghost/i2pd'}</span>
                                </div>
                            </div>

                            <div 
                                class="update-action-row" 
                                role="button"
                                tabindex="0"
                                on:click={onCheckUpdates}
                                on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onCheckUpdates()}
                            >
                                <div class="info-icon accent">{@html Icons.Refresh}</div>
                                <div class="info-content">
                                    <span class="info-value-fancy accent">Обновить приложение</span>
                                </div>
                                <div class="chevron">{@html Icons.ChevronRight}</div>
                            </div>
                        </div>

                        <div class="about-footer">
                            <p>© 2024-2026 TeleGhost Project. All rights reserved.</p>
                        </div>
                    </div>
                {/if}
            </div>
        </div>
    {/if}
</div>

<style>
    .settings-panel { flex: 1; display: flex; flex-direction: column; height: 100%; background: var(--bg-primary); }
    .settings-header { padding: 20px 40px; display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid var(--border); height: 80px; }
    .settings-header h2 { font-size: 24px; font-weight: 700; margin: 0; color: white; display: flex; align-items: center; }
    
    .btn-icon { background: rgba(255,255,255,0.05); border: 1px solid var(--border); color: white; border-radius: 14px; width: 44px; height: 44px; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: all 0.2s; padding: 0; }
    .btn-icon:hover { background: rgba(255,255,255,0.1); transform: rotate(90deg) scale(1.05); }
    .settings-header .btn-icon { order: -1; } /* For details view to keep back button left if needed, but wait user wants X right? No, ArrowLeft usually left. */
    
    .settings-view-menu .settings-header .btn-icon { order: 1; } /* X on the right */
    .settings-view-details .settings-header .btn-icon { order: -1; } /* Arrow on the left */
    
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
    .input-field { 
        width: 100%; 
        padding: 12px 16px; 
        background: #1a1a2e; 
        border: 1px solid rgba(255,255,255,0.1); 
        border-radius: 12px; 
        color: white; 
        outline: none; 
        font-size: 14px;
        transition: border-color 0.2s;
    }
    .input-field:focus { border-color: var(--accent); }
    
    select.input-field {
        appearance: none;
        background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='24' height='24' viewBox='0 0 24 24' fill='none' stroke='white' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E");
        background-repeat: no-repeat;
        background-position: right 12px center;
        background-size: 16px;
        padding-right: 40px;
    }
    select.input-field option { background: #1a1a2e; color: white; padding: 10px; }
    
    .btn-primary { background: var(--accent); color: white; padding: 14px 24px; border: none; border-radius: 16px; font-weight: 600; cursor: pointer; transition: all 0.2s; }
    .btn-primary:hover { opacity: 0.9; transform: translateY(-2px); box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3); }
    .btn-secondary { background: rgba(99, 102, 241, 0.1); border: 1px solid rgba(99, 102, 241, 0.3); color: #fff; padding: 12px 20px; border-radius: 14px; cursor: pointer; font-weight: 500; transition: all 0.2s; display: inline-flex; align-items: center; justify-content: center; gap: 8px; }
    .btn-secondary:hover { background: rgba(99, 102, 241, 0.2); border-color: var(--accent); transform: translateY(-1px); }
    
    .show-key-btn { 
        cursor: pointer; 
        font-weight: 700; 
        margin-top: 12px;
        width: 100%;
        border: 1px dashed rgba(99, 102, 241, 0.5);
    }
    .show-key-btn:hover {
        background: rgba(99, 102, 241, 0.15);
        border-style: solid;
    }

    .logout-btn { border-color: #f44336; color: #f44336; margin-top: 12px; }

    .info-box { padding: 24px; border-radius: 12px; margin-bottom: 24px; }
    .info-box.danger { background: rgba(255, 100, 100, 0.1); border: 1px solid rgba(255, 100, 100, 0.3); }
    .info-box h4 { margin: 0 0 12px; color: #ff6b6b; }

    .setting-item-box { background: var(--bg-secondary); padding: 20px; border-radius: 12px; }
    .flex-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
    .bg-box { background: var(--bg-secondary); padding: 16px; border-radius: 12px; }

    .destination-details { margin-bottom: 24px; cursor: pointer; background: rgba(99, 102, 241, 0.03); border-radius: 16px; border: 1px solid rgba(255,255,255,0.05); }
    .destination-details[open] { background: rgba(99, 102, 241, 0.05); }
    .destination-details summary { 
        padding: 16px 20px; list-style: none; display: flex; align-items: center; gap: 12px; 
        outline: none; transition: all 0.2s; font-weight: 600; color: #a29bfe;
    }
    .destination-details summary:hover { background: rgba(255,255,255,0.03); color: white; }
    .destination-details summary::-webkit-details-marker { display: none; }
    .destination-details summary::before { content: '🌐'; font-size: 14px; }
    .destination-details[open] summary { border-bottom: 1px solid rgba(255,255,255,0.05); }
    .hint-inline { font-size: 11px; opacity: 0.5; font-weight: 400; font-family: var(--font-main); margin-left: auto; }

    .destination-box { 
        display: flex; align-items: flex-start; gap: 10px; padding: 20px; 
        animation: slideDown 0.3s ease-out;
    }
    .destination-code { 
        flex: 1; font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 12px; 
        line-height: 1.6; opacity: 0.8; word-break: break-all; white-space: pre-wrap;
        max-height: 150px; overflow-y: auto; color: #a29bfe;
    }

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

    /* Redesigned About Section */
    .about-branding { text-align: center; margin-bottom: 40px; }
    .about-logo { 
        width: 100px; height: 100px; background: linear-gradient(135deg, #6366f1 0%, #a29bfe 100%); 
        border-radius: 30px; margin: 0 auto 20px; display: flex; align-items: center; justify-content: center;
        box-shadow: 0 15px 35px rgba(99, 102, 241, 0.3); color: white;
    }
    .icon-svg-lg { width: 50px; height: 50px; display: flex; align-items: center; justify-content: center; }
    .icon-svg-lg :global(svg) { width: 100%; height: 100%; }
    .about-title { font-size: 32px; font-weight: 800; margin-bottom: 8px; background: linear-gradient(90deg, #fff, #a29bfe); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
    .about-version { color: var(--text-secondary); font-size: 14px; font-weight: 500; opacity: 0.7; }

    .info-list { display: flex; flex-direction: column; gap: 12px; margin-bottom: 40px; }
    .info-item-fancy { 
        display: flex; align-items: center; gap: 16px; padding: 16px 20px; 
        background: rgba(255,255,255,0.03); border: 1px solid var(--border); border-radius: 16px;
    }
    .info-icon { 
        width: 40px; height: 40px; border-radius: 12px; background: rgba(255,255,255,0.05);
        display: flex; align-items: center; justify-content: center; color: var(--text-secondary);
    }
    .info-icon :global(svg) { width: 20px; height: 20px; }
    .info-icon.accent { background: rgba(99, 102, 241, 0.1); color: var(--accent); }
    
    .info-content { display: flex; flex-direction: column; gap: 2px; flex: 1; }
    .info-label-fancy { font-size: 11px; text-transform: uppercase; letter-spacing: 1px; color: var(--text-secondary); font-weight: 700; }
    .info-value-fancy { font-size: 15px; color: var(--text-primary); font-weight: 500; }
    .info-value-fancy.accent { color: var(--accent); font-weight: 700; }

    .update-action-row { 
        display: flex; align-items: center; gap: 16px; padding: 16px 20px; 
        background: rgba(99, 102, 241, 0.05); border: 1px solid rgba(99, 102, 241, 0.1); 
        border-radius: 16px; cursor: pointer; transition: all 0.2s;
    }
    .update-action-row:hover { background: rgba(99, 102, 241, 0.1); border-color: rgba(99, 102, 241, 0.3); transform: translateY(-2px); }
    .chevron { color: var(--text-secondary); opacity: 0.5; }
    .chevron :global(svg) { width: 18px; height: 18px; }

    .about-footer { text-align: center; color: var(--text-secondary); font-size: 12px; opacity: 0.4; margin-top: auto; }

    @keyframes float {
        0%, 100% { transform: translateY(0); }
        50% { transform: translateY(-10px); }
    }
    .animate-float { animation: float 6s ease-in-out infinite; }
</style>
