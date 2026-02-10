<script>
    import { Icons } from '../Icons.js';
    import { getInitials } from '../utils.js';

    // Confirm Modal
    export let showConfirmModal = false;
    export let confirmModalTitle = '';
    export let confirmModalText = '';
    export let onConfirm;
    export let onCancelConfirm;

    // Folder Modal
    export let showFolderModal = false;
    export let isEditingFolder = false;
    export let folderName = '';
    export let folderIcon = '📁';
    export let onSaveFolder;
    export let onCancelFolder;

    // Contact Profile Modal
    export let showContactProfile = false;
    export let contact = null;
    export let onCloseContactProfile;

    export let onCancelAddToFolder;
    
    // Add/Search Contact Modal
    export let showAddContact = false;
    export let onAddContact;
    export let onCancelAddContact;
    export let addContactName = '';
    export let addContactAddress = '';

    const handleAddContact = () => {
        onAddContact();
    };

    // Show Seed Modal
    export let showSeedModal = false;
    export let mnemonic = '';
    export let onCloseSeed;

</script>

<!-- Custom Confirm Modal -->
{#if showConfirmModal}
<div class="modal-backdrop animate-fade-in" on:click|self={onCancelConfirm}>
    <div class="modal-content animate-slide-down" style="max-width: 400px;">
        <div class="modal-header">
            <h3>{confirmModalTitle}</h3>
            <button class="btn-icon" on:click={onCancelConfirm}><div class="icon-svg">{@html Icons.X}</div></button>
        </div>
        <div class="modal-body">
            <p style="color: var(--text-secondary); margin-bottom: 20px;">{confirmModalText}</p>
        </div>
        <div class="modal-footer">
            <button class="btn-small btn-glass" on:click={onCancelConfirm}>Отмена</button>
            <button class="btn-small btn-primary" on:click={onConfirm}>OK</button>
        </div>
    </div>
</div>
{/if}

<!-- Add/Edit Folder Modal -->
{#if showFolderModal}
<div class="modal-backdrop animate-fade-in" on:click|self={onCancelFolder}>
    <div class="modal-content animate-slide-down" style="max-width: 400px;">
        <div class="modal-header">
            <h3>{isEditingFolder ? 'Редактировать папку' : 'Новая папка'}</h3>
            <button class="btn-icon" on:click={onCancelFolder}><div class="icon-svg">{@html Icons.X}</div></button>
        </div>
        <div class="modal-body">
            <label class="form-label">Название
                <input type="text" bind:value={folderName} class="input-field" placeholder="Напр: Работа, Семья" />
            </label>
            <label class="form-label" style="margin-top: 16px;">Иконка (Emoji)
                <input type="text" bind:value={folderIcon} class="input-field" style="font-size: 24px; text-align: center;" />
            </label>
        </div>
        <div class="modal-footer">
            <button class="btn-small btn-glass" on:click={onCancelFolder}>Отмена</button>
            <button class="btn-small btn-primary" on:click={onSaveFolder}>Сохранить</button>
        </div>
    </div>
</div>
{/if}

<!-- Contact Profile Modal -->
{#if showContactProfile && contact}
<div class="modal-backdrop animate-fade-in" on:click|self={onCloseContactProfile}>
    <div class="modal-content animate-slide-down">
        <div class="modal-header">
            <h3>Профиль контакта</h3>
            <button class="btn-icon" on:click={onCloseContactProfile}><div class="icon-svg">{@html Icons.X}</div></button>
        </div>
        <div class="modal-body" style="text-align: center; padding: 20px 0;">
            <div class="profile-avatar-large" style="width: 100px; height: 100px; margin: 0 auto 20px; background: var(--accent); border-radius: 50%; overflow: hidden; display: flex; align-items: center; justify-content: center; font-size: 40px; color: white;">
                {#if contact.avatar}<img src={contact.avatar} alt="av" style="width:100%;height:100%;object-fit:cover;"/>{:else}{getInitials(contact.nickname)}{/if}
            </div>
            <h2 style="margin-bottom: 4px;">{contact.nickname}</h2>
            <p style="color: var(--text-secondary); font-size: 14px; margin-bottom: 24px;">{contact.isOnline ? 'В сети' : 'Оффлайн'}</p>
            
            <div style="text-align: left; background: var(--bg-input); padding: 16px; border-radius: 12px;">
                <label style="font-size: 12px; color: var(--text-secondary); display: block; margin-bottom: 4px;">I2P Адрес</label>
                <code style="font-size: 11px; word-break: break-all; opacity: 0.8;">{contact.i2pAddress}</code>
            </div>
        </div>
        <div class="modal-footer">
            <button class="btn-primary full-width" on:click={() => { navigator.clipboard.writeText(contact.i2pAddress); }}>Скопировать адрес</button>
        </div>
    </div>
</div>
{/if}

<!-- Add Contact Modal -->
{#if showAddContact}
<div class="modal-backdrop animate-fade-in" on:click|self={onCancelAddContact}>
    <div class="modal-content animate-slide-down" style="max-width: 450px;">
        <div class="modal-header">
            <h3>Добавить контакт</h3>
            <button class="btn-icon" on:click={onCancelAddContact}><div class="icon-svg">{@html Icons.X}</div></button>
        </div>
        <div class="modal-body">
            <div class="form-group">
                <label class="form-label">Никнейм
                    <input type="text" bind:value={addContactName} class="input-field" placeholder="Напр: Иван" />
                </label>
            </div>
            <div class="form-group" style="margin-top: 16px;">
                <label class="form-label">I2P Адрес (Full Destination)
                    <textarea bind:value={addContactAddress} class="input-field" rows="4" placeholder="Введите полный I2P адрес..."></textarea>
                </label>
            </div>
        </div>
        <div class="modal-footer">
            <button class="btn-small btn-glass" on:click={onCancelAddContact}>Отмена</button>
            <button class="btn-small btn-primary" on:click={handleAddContact}>Добавить</button>
        </div>
    </div>
</div>
{/if}

<!-- Show Seed Modal -->
{#if showSeedModal}
<div class="modal-backdrop animate-fade-in" on:click|self={onCloseSeed}>
    <div class="modal-content animate-slide-down" style="max-width: 450px; background: #1a1a2e; border: 1px solid rgba(255, 100, 100, 0.2);">
        <div class="modal-header">
            <h3 style="color: #ff6b6b;">🔐 Ваш секретный ключ</h3>
            <button class="btn-icon" on:click={onCloseSeed}><div class="icon-svg">{@html Icons.X}</div></button>
        </div>
        <div class="modal-body">
            <p class="warning-text" style="color: #ff6b6b; font-size: 13px; margin-bottom: 20px; background: rgba(255,100,100,0.1); padding: 12px; border-radius: 8px;">
                ⚠️ Никогда не передавайте эти слова третьим лицам! Тот, у кого есть фраза, имеет ПОЛНЫЙ доступ к вашему аккаунту.
            </p>
            
            <div class="mnemonic-grid" style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin-bottom: 24px;">
                {#each mnemonic.split(' ') as word, i}
                    <div class="mnemonic-word" style="background: rgba(0,0,0,0.2); padding: 8px; border-radius: 8px; display: flex; align-items: center; gap: 8px; border: 1px solid rgba(255,255,255,0.05);">
                        <span class="word-index" style="font-size: 10px; opacity: 0.5;">{i+1}</span>
                        <span class="word-text" style="font-size: 13px; font-weight: 500;">{word}</span>
                    </div>
                {/each}
            </div>
            
            <button class="btn-glass full-width" on:click={() => { navigator.clipboard.writeText(mnemonic); }} style="padding: 12px;">
                📋 Скопировать всё
            </button>
        </div>
        <div class="modal-footer">
            <button class="btn-primary full-width accent-btn clickable-btn" on:click|preventDefault|stopPropagation={onCloseSeed}>Я всё сохранил(а)</button>
        </div>
    </div>
</div>
{/if}

<style>
    .modal-backdrop { position: fixed; top:0; left:0; width:100vw; height:100vh; background:rgba(0,0,0,0.8); backdrop-filter:blur(5px); display:flex; align-items:center; justify-content:center; z-index: 1000; }
    .modal-content { background: var(--bg-secondary); border-radius: 16px; padding: 24px; width: 90%; max-width: 500px; box-shadow: 0 20px 60px rgba(0,0,0,0.5); border: 1px solid var(--border); }
    .modal-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; }
    .modal-header h3 { margin: 0; font-size: 20px; color: white; }
    
    .form-label { display: block; font-size: 14px; color: var(--text-secondary); margin-bottom: 8px; }
    .input-field { width: 100%; padding: 12px; background: var(--bg-input); border: 1px solid var(--border); border-radius: 10px; color: white; outline: none; }
    
    .modal-footer { display: flex; gap: 12px; margin-top: 24px; }
    .btn-small { padding: 10px 20px; border-radius: 12px; font-weight: 600; cursor: pointer; border: none; transition: all 0.2s; }
    .btn-small:hover { opacity: 0.9; transform: translateY(-1px); }
    .btn-primary { background: var(--accent); color: white; flex: 1; border: none; }
    .accent-btn { background: #6366f1 !important; }
    .btn-glass { background: rgba(255,255,255,0.05); color: #a0a0ba; border: 1px solid rgba(255,255,255,0.1); }
    .btn-glass:hover { background: rgba(255,255,255,0.1); color: white; }
    
    .full-width { width: 100%; }
    .icon-svg { width: 24px; height: 24px; }
    .btn-icon { border: none; color: white; cursor: pointer; background: rgba(255,255,255,0.05); width: 32px; height: 32px; border-radius: 8px; display: flex; align-items: center; justify-content: center; transition: all 0.2s; padding: 0;}
    .btn-icon:hover { background: rgba(255,255,255,0.1); }
    .clickable-btn { cursor: pointer !important; position: relative; z-index: 10002; transition: all 0.2s; }
    .clickable-btn:hover { filter: brightness(1.1); transform: translateY(-1px); }
</style>
