<script>
    import { Icons } from '../Icons.js';

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

    // Add Chat to Folder Modal
    export let showAddToFolder = false;
    export let folders = [];
    export let onAddToFolder;
    export let onCancelAddToFolder;

</script>

<!-- Custom Confirm Modal -->
{#if showConfirmModal}
<div class="modal-backdrop animate-fade-in" on:click|self={onCancelConfirm}>
    <div class="modal-content animate-slide-down" style="max-width: 400px;">
        <div class="modal-header">
            <h3>{confirmModalTitle}</h3>
        </div>
        <div class="modal-body">
            <p style="color: var(--text-secondary); margin-bottom: 20px;">{confirmModalText}</p>
        </div>
        <div class="modal-footer">
            <button class="btn-small btn-secondary" on:click={onCancelConfirm}>Отмена</button>
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
            <button class="btn-small btn-secondary" on:click={onCancelFolder}>Отмена</button>
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
                {#if contact.avatar}<img src={contact.avatar} alt="av" style="width:100%;height:100%;object-fit:cover;"/>{:else}{contact.nickname[0].toUpperCase()}{/if}
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

<style>
    .modal-backdrop { position: fixed; top:0; left:0; width:100vw; height:100vh; background:rgba(0,0,0,0.8); backdrop-filter:blur(5px); display:flex; align-items:center; justify-content:center; z-index: 1000; }
    .modal-content { background: var(--bg-secondary); border-radius: 16px; padding: 24px; width: 90%; max-width: 500px; box-shadow: 0 20px 60px rgba(0,0,0,0.5); border: 1px solid var(--border); }
    .modal-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; }
    .modal-header h3 { margin: 0; font-size: 20px; color: white; }
    
    .form-label { display: block; font-size: 14px; color: var(--text-secondary); margin-bottom: 8px; }
    .input-field { width: 100%; padding: 12px; background: var(--bg-input); border: 1px solid var(--border); border-radius: 10px; color: white; outline: none; }
    
    .modal-footer { display: flex; gap: 12px; margin-top: 24px; }
    .btn-small { padding: 10px 20px; border-radius: 8px; font-weight: 600; cursor: pointer; border: none; }
    .btn-primary { background: var(--accent); color: white; flex: 1; }
    .btn-secondary { background: rgba(255,255,255,0.05); color: white; border: 1px solid var(--border); }
    
    .full-width { width: 100%; }
    .icon-svg { width: 24px; height: 24px; }
    .btn-icon { background: transparent; border: none; color: white; cursor: pointer; }
</style>
