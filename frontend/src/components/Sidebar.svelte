<script>
    import { Icons } from '../Icons.js';
    import { getInitials, formatTime, getStatusColor, getStatusText } from '../utils.js';
    import { writable } from 'svelte/store';

    export let isMobile;
    export let contacts = [];
    export let folders = [];
    export let activeFolderId;
    export let searchQuery;
    export let networkStatus;
    export let showSettings;
    export let sidebarWidth;
    export let isResizing;
    export let selectedContact;

    export let onSelectContact;
    export let onContextMenu;
    export let onToggleSettings;
    export let onStartResize;
    export let onOpenAddContact;
    export let onAddContactFromClipboard;
    export let onCopyDestination;
    export let onSelectFolder;
    export let onEditFolder;
    export let onCreateFolder;

    let longPressTimer;

    $: filteredContacts = (contacts || []).filter(c => {
        if (!c || !c.Nickname) return false;
        
        // Фильтрация по папкам
        if (activeFolderId !== 'all') {
            const folder = (folders || []).find(f => f.ID === activeFolderId);
            if (folder && folder.ChatIDs && !folder.ChatIDs.includes(c.ChatID)) {
                return false;
            }
        }

        const query = (searchQuery || "").toLowerCase();
        const nickname = (c.Nickname || "").toLowerCase();
        const lastMsg = (c.LastMessage || "").toLowerCase();
        return nickname.includes(query) || lastMsg.includes(query);
    });

    $: uiFolders = [
        { ID: 'all', Name: 'Все', Icon: Icons.Chat },
        ...([...(folders || [])].sort((a, b) => (a?.position || 0) - (b?.position || 0))),
        { id: 'add', name: 'Создать', icon: Icons.Plus }
    ];

    function handleTouchStart(item, type, e) {
        longPressTimer = setTimeout(() => {
            if (type === 'contact') {
                onContextMenu(e, item);
            } else if (type === 'folder') {
                onEditFolder(item);
            }
        }, 700);
    }

    function handleTouchEnd() {
        clearTimeout(longPressTimer);
    }

    function handleFolderClick(folder) {
        if (folder.ID === 'add') {
            onCreateFolder();
        } else {
            onSelectFolder(folder.ID);
        }
    }
</script>

{#if !isMobile}
    <!-- Folders Rail -->
    <div class="folders-rail">
        <div 
            class="rail-button" 
            class:active={showSettings} 
            role="button"
            tabindex="0"
            on:click={onToggleSettings}
            on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onToggleSettings()}
        >
            <div class="icon-svg">{@html Icons.Menu}</div>
        </div>

        <div class="folders-list">
            {#each uiFolders as folder}
                <div 
                    class="folder-item" 
                    class:active={!showSettings && activeFolderId === folder.ID && folder.ID !== 'add'} 
                    role="button"
                    tabindex="0"
                    on:click={() => handleFolderClick(folder)}
                    on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && handleFolderClick(folder)}
                    on:contextmenu|preventDefault={(e) => folder.ID !== 'all' && folder.ID !== 'add' && onEditFolder(folder)}
                    on:touchstart={(e) => folder.ID !== 'all' && folder.ID !== 'add' && handleTouchStart(folder, 'folder', e)}
                    on:touchend={handleTouchEnd}
                    on:touchmove={handleTouchEnd}
                    title={folder.Name}
                    style={folder.ID === 'add' ? 'margin-top: 10px; opacity: 0.7;' : ''}
                >
                    <div class="folder-icon">{@html folder.Icon}</div>
                    <div class="folder-name">{folder.Name}</div>
                </div>
            {/each}
        </div>
    </div>

    <!-- Sidebar (Resizable) -->
    <div class="sidebar" style="width: {sidebarWidth}px; min-width: 240px; flex: none; display: flex; flex-direction: column;">
        <div class="sidebar-header" style="padding: 10px; background: var(--bg-secondary);">
            <div class="search-input-wrapper">
                <span class="search-icon"><div class="icon-svg-sm">{@html Icons.Search}</div></span>
                <input type="text" placeholder="Поиск" bind:value={searchQuery} />
            </div>
        </div>
        
        <div class="sidebar-actions">
           <button class="btn-primary" on:click={onOpenAddContact}>
              <div class="icon-svg-sm">{@html Icons.MessageSquarePlus}</div>
              <span>Новый чат</span>
           </button>
        </div>
        
        <!-- Network Status -->
        <div class="network-status" style="background: {getStatusColor(networkStatus)}15">
            <div class="status-dot" class:animate-pulse={networkStatus === 'connecting'} style="background: {getStatusColor(networkStatus)}"></div>
            <span>{getStatusText(networkStatus)}</span>
        </div>
        
        <!-- Contacts List -->
        <div class="contacts-list">
            {#each filteredContacts as contact}
                <div 
                    class="contact-item animate-card" 
                    class:selected={selectedContact && selectedContact.ID === contact.ID} 
                    on:click={() => onSelectContact(contact)}
                    on:contextmenu|preventDefault={(e) => onContextMenu(e, contact)}
                    on:touchstart={(e) => handleTouchStart(contact, 'contact', e)}
                    on:touchend={handleTouchEnd}
                    on:touchmove={handleTouchEnd}
                    on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onSelectContact(contact)}
                    tabindex="0"
                    role="button"
                >
                    <div class="contact-avatar" style="background: var(--accent);">
                        {#if contact.Avatar}
                            <img src={contact.Avatar} alt="av"/>
                        {:else}
                            {getInitials(contact.Nickname)}
                        {/if}
                    </div>
                    <div class="contact-info">
                        <div class="contact-name">{contact.Nickname}</div>
                        <div class="contact-last">{contact.LastMessage || 'Нет сообщений'}</div>
                    </div>
                </div>
            {/each}
            
            {#if contacts.length === 0}
                <div class="no-contacts">
                    <div class="no-contacts-icon"><div class="icon-svg" style="width:48px;height:48px;">{@html Icons.Ghost}</div></div>
                    <p>Нет контактов</p>
                    <p class="hint">Нажмите + чтобы добавить</p>
                </div>
            {/if}
        </div>
        
        <!-- My Destination -->
        <div class="my-destination">
            <button class="btn-copy" on:click={onCopyDestination}>
                <div class="icon-svg-sm">{@html Icons.Copy}</div>
                <span>Копировать мой I2P адрес</span>
            </button>
        </div>
    </div>
    
    <div class="resizer" on:mousedown={onStartResize} class:resizing={isResizing}></div>
{/if}

<style>
    /* Add sidebar styles from App.svelte */
    .folders-rail {
        width: 72px;
        background: var(--bg-tertiary, #11111b);
        display: flex;
        flex-direction: column;
        align-items: center;
        padding-top: 10px;
        flex-shrink: 0;
        z-index: 100;
        border-right: 1px solid var(--border, rgba(255,255,255,0.05));
    }

    .rail-button {
        width: 48px;
        height: 48px;
        border-radius: 12px;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
        margin-bottom: 20px;
        color: var(--text-secondary);
        transition: all 0.2s;
    }
    .rail-button:hover, .rail-button.active {
        background: rgba(255,255,255,0.1);
        color: white;
    }

    .folder-item {
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        width: 64px;
        height: 64px;
        margin-bottom: 8px;
        cursor: pointer;
        border-radius: 12px;
        transition: background 0.2s;
        gap: 2px;
    }
    .folder-item:hover, .folder-item.active {
        background: rgba(255, 255, 255, 0.1);
    }
    .folder-icon { font-size: 24px; }
    .folder-name {
        font-size: 10px; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: center; color: var(--text-secondary);
    }

    .sidebar { background: var(--bg-secondary, #1e1e2e); border-right: 1px solid var(--border); overflow: hidden; }
    .search-input-wrapper { background: var(--bg-input, #0c0c14); border-radius: 18px; padding: 8px 12px; display: flex; align-items: center; gap: 8px; }
    .search-input-wrapper input { background: transparent; border: none; color: white; width: 100%; font-size: 14px; outline: none; }
    
    .sidebar-actions { padding: 0 10px 10px; }
    .btn-primary { 
        width: 100%; display: flex; align-items: center; justify-content: center; gap: 8px; padding: 10px; background: var(--accent, #6366f1); color: white; border: none; border-radius: 14px; cursor: pointer; font-weight: 600; transition: all 0.2s;
    }
    .btn-primary:hover { transform: translateY(-1px); filter: brightness(1.1); box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3); }

    .network-status {
        padding: 10px 16px; display: flex; align-items: center; gap: 10px; font-size: 12px; color: var(--text-primary); margin: 0 10px 10px; border-radius: 8px;
    }
    .status-dot { width: 8px; height: 8px; border-radius: 50%; }
    .animate-pulse { animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite; }
    @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .5; } }

    .contacts-list { flex: 1; overflow-y: auto; }
    .contact-item {
        padding: 12px 16px; cursor: pointer; display: flex; gap: 12px; align-items: center; transition: background 0.2s; border-radius: 12px; margin: 2px 8px;
    }
    .contact-item:hover { background: rgba(255, 255, 255, 0.05); }
    .contact-item.selected { background: rgba(108, 92, 231, 0.15); }

    .contact-avatar { width: 48px; height: 48px; border-radius: 50%; display: flex; align-items: center; justify-content: center; color: white; font-weight: bold; flex-shrink: 0; overflow: hidden; }
    .contact-avatar img { width: 100%; height: 100%; object-fit: cover; }

    .contact-info { flex: 1; min-width: 0; }
    .contact-name { font-weight: 600; font-size: 15px; color: white; margin-bottom: 2px; }
    .contact-last { font-size: 13px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

    .no-contacts { padding: 40px 20px; text-align: center; color: var(--text-secondary); }
    .no-contacts-icon { margin-bottom: 16px; opacity: 0.5; display: flex; justify-content: center; }
    .hint { font-size: 12px; margin-top: 4px; opacity: 0.7; }

    .my-destination { padding: 10px; border-top: 1px solid var(--border); }
    .btn-copy {
        width: 100%; display: flex; align-items: center; justify-content: center; gap: 8px; padding: 10px; background: rgba(255, 255, 255, 0.05); color: var(--text-primary); border: 1px solid rgba(255, 255, 255, 0.05); border-radius: 12px; cursor: pointer; font-size: 13px; transition: background 0.2s;
    }
    .btn-copy:hover { background: rgba(255, 255, 255, 0.1); }

    .resizer { width: 4px; cursor: col-resize; background: transparent; transition: background 0.2s; flex-shrink: 0; }
    .resizer:hover, .resizer.resizing { background: var(--accent); }
    
    .icon-svg { width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; }
    .icon-svg :global(svg) { width: 100%; height: 100%; }
    .icon-svg-sm { width: 18px; height: 18px; display: flex; align-items: center; justify-content: center; }
    .icon-svg-sm :global(svg) { width: 100%; height: 100%; }
</style>
