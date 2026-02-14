<script>
    import { Icons } from '../Icons.js';
    import { getInitials, formatTime, getStatusColor, getStatusText, getAvatarGradient } from '../utils.js';
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
    export let identity;
    export let unreadCount = 0;
    export let pinnedChats = [];

    export let onSelectContact;
    export let onContextMenu;
    export let onToggleSettings;
    export let onStartResize;
    export let onOpenAddContact;
    export let onAddContactFromClipboard;
    export let onCopyDestination;
    export let onOpenMyQR;
    export let onSelectFolder;
    export let onEditFolder;
    export let onCreateFolder;
    export let onFolderContextMenu;

    let longPressTimer;

    $: filteredContacts = (contacts || []).filter(c => {
        if (!c || !c.Nickname) return false;
        
        // Фильтрация по папкам
        if (activeFolderId && activeFolderId !== 'all') {
            const folder = (folders || []).find(f => (f.ID || f.id) === activeFolderId);
            if (folder) {
                const chatIds = folder.ChatIDs || folder.chat_ids || [];
                if (!chatIds.includes(c.ID)) {
                    return false;
                }
            }
        }

        const query = (searchQuery || "").toLowerCase();
        const nickname = (c.Nickname || "").toLowerCase();
        const lastMsg = (c.LastMessage || "").toLowerCase();
        return nickname.includes(query) || lastMsg.includes(query);
    });

    $: sortedContacts = [...filteredContacts].sort((a, b) => {
        const aIndex = pinnedChats.indexOf(a.ID);
        const bIndex = pinnedChats.indexOf(b.ID);
        
        if (aIndex !== -1 && bIndex !== -1) {
            return aIndex - bIndex; // Both pinned, sort by index
        }
        if (aIndex !== -1) return -1; // a pinned
        if (bIndex !== -1) return 1;  // b pinned
        
        // Keep original order otherwise (usually by last message time if backend provides it sorted, or ID)
        return 0;
    });

    $: totalUnread = (contacts || []).reduce((sum, c) => sum + (c.UnreadCount || 0), 0);

    $: uiFolders = [
        { ID: 'all', Name: 'Все', Icon: Icons.Chat, UnreadCount: totalUnread },
        ...([...(folders || [])].sort((a, b) => ((a.Position ?? a.position) || 0) - ((b.Position ?? b.position) || 0))),
        { ID: 'add', Name: 'Создать', Icon: Icons.Plus, UnreadCount: 0 }
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
        const id = folder.ID || folder.id;
        if (id === 'add') {
            onCreateFolder();
        } else {
            onSelectFolder(id);
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
                    class:active={!showSettings && activeFolderId === (folder.ID || folder.id) && (folder.ID || folder.id) !== 'add'} 
                    role="button"
                    tabindex="0"
                    on:click={() => handleFolderClick(folder)}
                    on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && handleFolderClick(folder)}
                    on:contextmenu|preventDefault={(e) => (folder.ID || folder.id) !== 'all' && (folder.ID || folder.id) !== 'add' && onFolderContextMenu(e, folder)}
                    on:touchstart={(e) => (folder.ID || folder.id) !== 'all' && (folder.ID || folder.id) !== 'add' && handleTouchStart(folder, 'folder', e)}
                    on:touchend={handleTouchEnd}
                    on:touchmove={handleTouchEnd}
                    title={folder.Name || folder.name}
                    style={(folder.ID || folder.id) === 'add' ? 'margin-top: 10px; opacity: 0.7;' : 'position: relative;'}
                >
                    <div class="folder-icon">{@html folder.Icon || folder.icon}</div>
                    <div class="folder-name">{folder.Name || folder.name}</div>
                    {#if (folder.UnreadCount || folder.unread_count) > 0 && (folder.ID || folder.id) !== 'add'}
                        <div class="folder-unread-badge">{(folder.UnreadCount || folder.unread_count) > 99 ? '99+' : (folder.UnreadCount || folder.unread_count)}</div>
                    {/if}
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
            <!-- Favorites Chat -->
            {#if !searchQuery || 'избранное'.includes(searchQuery.toLowerCase()) || 'saved messages'.includes(searchQuery.toLowerCase()) || 'favorites'.includes(searchQuery.toLowerCase())}
                <div 
                    class="contact-item favorites-item animate-card" 
                    class:selected={selectedContact && (selectedContact.IsFavorites || selectedContact.ID === identity)} 
                    on:click={() => onSelectContact({ID: identity, Nickname: 'Избранное', IsFavorites: true, ChatID: identity})}
                    on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onSelectContact({ID: identity, Nickname: 'Избранное', IsFavorites: true, ChatID: identity})}
                    tabindex="0"
                    role="button"
                >
                    <div class="contact-avatar favorites-avatar" style="background: linear-gradient(135deg, #6366f1 0%, #a78bfa 100%);">
                        <div class="icon-svg-sm" style="color: white; width: 20px; height: 20px;">{@html Icons.Bookmark}</div>
                    </div>
                    <div class="contact-info">
                        <div class="contact-name">Избранное</div>
                        <div class="contact-last">Сохранённые сообщения</div>
                    </div>
                </div>
            {/if}

            {#each sortedContacts as contact}
                <div 
                    class="contact-item animate-card" 
                    class:selected={selectedContact && selectedContact.ID === contact.ID} 
                    class:pinned={pinnedChats.includes(contact.ID)}
                    on:click={() => onSelectContact(contact)}
                    on:contextmenu|preventDefault={(e) => onContextMenu(e, contact)}
                    on:touchstart={(e) => handleTouchStart(contact, 'contact', e)}
                    on:touchend={handleTouchEnd}
                    on:touchmove={handleTouchEnd}
                    on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onSelectContact(contact)}
                    tabindex="0"
                    role="button"
                >
                    <div class="contact-avatar" style="background: {getAvatarGradient(contact.Nickname)};">
                        {#if contact.Avatar}
                            <img src={contact.Avatar} alt="av"/>
                        {:else}
                            {getInitials(contact.Nickname)}
                        {/if}
                    </div>
                    <div class="contact-info">
                        <div class="contact-header">
                            <div class="contact-name">
                                {contact.Nickname}
                                {#if pinnedChats.includes(contact.ID)}
                                    <span class="pin-icon">{@html Icons.Pin}</span>
                                {/if}
                            </div>
                            <span class="contact-time">{contact.LastMessageTime ? formatTime(contact.LastMessageTime) : ''}</span>
                        </div>
                        <div class="contact-last">{contact.LastMessage || 'Нет сообщений'}</div>
                    </div>
                    {#if contact.UnreadCount > 0}
                        <div class="contact-unread-badge">{contact.UnreadCount > 99 ? '99+' : contact.UnreadCount}</div>
                    {/if}
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
            <button class="btn-copy" on:click={onOpenMyQR}>
                <div class="icon-svg-sm">{@html Icons.QrCode}</div>
                <span>Мой I2P адрес</span>
            </button>
        </div>
    </div>
    
    <div class="resizer" on:mousedown={onStartResize} class:resizing={isResizing}></div>
{:else}
    <!-- Mobile Sidebar -->
    <div class="mobile-sidebar">
        <!-- Mobile Header -->
        <div class="mobile-header">
            <button class="mobile-menu-btn" on:click={onToggleSettings} style="position: relative;">
                <div class="icon-svg">{@html Icons.Menu}</div>
            </button>
            <h1 class="mobile-title">TeleGhost</h1>
            <div class="mobile-network-dot" style="background: {getStatusColor(networkStatus)}" title={getStatusText(networkStatus)}></div>
        </div>
        
        <!-- Mobile Search -->
        <div class="mobile-search">
            <div class="search-input-wrapper">
                <span class="search-icon"><div class="icon-svg-sm">{@html Icons.Search}</div></span>
                <input type="text" placeholder="Поиск" bind:value={searchQuery} />
            </div>
        </div>

        <!-- Mobile Folders Bar -->
        <div class="mobile-folders-bar">
            {#each uiFolders as folder}
                <button 
                    class="mobile-folder-chip"
                    class:active={activeFolderId === (folder.ID || folder.id) && (folder.ID || folder.id) !== 'add'}
                    on:click={() => handleFolderClick(folder)}
                    on:touchstart={(e) => (folder.ID || folder.id) !== 'all' && (folder.ID || folder.id) !== 'add' && handleTouchStart(folder, 'folder', e)}
                    on:touchend={handleTouchEnd}
                    style="position: relative;"
                >
                    <span class="mobile-folder-icon">{@html folder.Icon || folder.icon}</span>
                    <span>{folder.Name || folder.name}</span>
                    {#if (folder.UnreadCount || folder.unread_count) > 0 && (folder.ID || folder.id) !== 'add'}
                        <span class="mobile-folder-badge">{(folder.UnreadCount || folder.unread_count) > 99 ? '99+' : (folder.UnreadCount || folder.unread_count)}</span>
                    {/if}
                </button>
            {/each}
        </div>

        <!-- Mobile Contacts List -->
        <div class="contacts-list mobile-contacts">
            <!-- Mobile Favorites Chat -->
            {#if !searchQuery || 'избранное'.includes(searchQuery.toLowerCase()) || 'saved messages'.includes(searchQuery.toLowerCase()) || 'favorites'.includes(searchQuery.toLowerCase())}
                <div 
                    class="contact-item favorites-item animate-card"
                    class:selected={selectedContact && (selectedContact.IsFavorites || selectedContact.ID === identity)}
                    on:click={() => onSelectContact({ID: identity, Nickname: 'Избранное', IsFavorites: true, ChatID: identity})}
                    tabindex="0"
                    role="button"
                >
                    <div class="contact-avatar favorites-avatar" style="background: linear-gradient(135deg, #6366f1 0%, #a78bfa 100%);">
                        <div class="icon-svg-sm" style="color: white; width: 20px; height: 20px;">{@html Icons.Bookmark}</div>
                    </div>
                    <div class="contact-info">
                        <div class="contact-name">Избранное</div>
                        <div class="contact-last">Сохранённые сообщения</div>
                    </div>
                </div>
            {/if}

            {#each filteredContacts as contact}
                <div 
                    class="contact-item animate-card"
                    class:selected={selectedContact && selectedContact.ID === contact.ID}
                    on:click={() => onSelectContact(contact)}
                    on:contextmenu|preventDefault={(e) => onContextMenu(e, contact)}
                    on:touchstart={(e) => handleTouchStart(contact, 'contact', e)}
                    on:touchend={handleTouchEnd}
                    on:touchmove={handleTouchEnd}
                    tabindex="0"
                    role="button"
                >
                    <div class="contact-avatar" style="background: {getAvatarGradient(contact.Nickname)};">
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
                    {#if contact.UnreadCount > 0}
                        <div class="contact-unread-badge">{contact.UnreadCount > 99 ? '99+' : contact.UnreadCount}</div>
                    {/if}
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
        
        <!-- Mobile Bottom Actions -->
        <div class="mobile-bottom-bar">
            <button class="mobile-action-btn" on:click={onOpenAddContact}>
                <div class="icon-svg">{@html Icons.MessageSquarePlus}</div>
                <span>Новый чат</span>
            </button>
            <button class="mobile-action-btn" on:click={() => {
                // If onOpenMyQR is just opening modal, maybe we want direct copy?
                // The user asked "cannot copy own qr code/address".
                // Let's open QR modal (onOpenMyQR) BUT ensure QR modal has copy button.
                // Or successfuly adding a copy action here too.
                // Let's do both: Open QR, but also long press to copy? 
                // Simple: just open QR, but verify QR modal has copy.
                // Wait, user said "cannot copy". 
                // Let's add a copy button TO the QR modal. 
                // For now, let's make this button open QR, and separately ensure QR modal has copy.
                onOpenMyQR();
            }}>
                <div class="icon-svg">{@html Icons.QrCode}</div>
                <span>Мой адрес</span>
            </button>
        </div>
    </div>
{/if}

<style>
    /* === Desktop Styles === */
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
        position: relative;
    }
    .rail-button:hover, .rail-button.active {
        background: rgba(255,255,255,0.1);
        color: white;
    }

    .unread-badge {
        position: absolute;
        top: -4px;
        right: -4px;
        background: #ff4757;
        color: white;
        font-size: 10px;
        font-weight: 700;
        min-width: 18px;
        height: 18px;
        border-radius: 9px;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0 4px;
        box-shadow: 0 2px 8px rgba(255, 71, 87, 0.4);
        animation: badge-pulse 2s ease-in-out infinite;
    }

    .mobile-badge {
        top: 2px;
        right: 2px;
        font-size: 9px;
        min-width: 16px;
        height: 16px;
        border-radius: 8px;
    }

    @keyframes badge-pulse {
        0%, 100% { transform: scale(1); }
        50% { transform: scale(1.1); }
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

    .folder-unread-badge {
        position: absolute;
        top: 4px;
        right: 4px;
        background: #ff4757;
        color: white;
        font-size: 9px;
        font-weight: 700;
        min-width: 16px;
        height: 16px;
        border-radius: 8px;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0 4px;
        box-shadow: 0 2px 6px rgba(255, 71, 87, 0.4);
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

    .contact-unread-badge {
        background: #ff4757;
        color: white;
        font-size: 11px;
        font-weight: 700;
        min-width: 20px;
        height: 20px;
        border-radius: 10px;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0 6px;
        flex-shrink: 0;
        box-shadow: 0 2px 6px rgba(255, 71, 87, 0.3);
    }

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

    /* === Mobile Styles === */
    .mobile-sidebar {
        width: 100%;
        height: 100dvh;
        display: flex;
        flex-direction: column;
        background: var(--bg-primary, #0c0c14);
        overflow: hidden;
    }

    .mobile-header {
        display: flex;
        align-items: center;
        gap: 12px;
        padding: 12px 16px;
        padding-top: calc(12px + env(safe-area-inset-top));
        background: var(--bg-secondary, #1e1e2e);
        border-bottom: 1px solid var(--border);
    }

    .mobile-menu-btn {
        background: transparent;
        border: none;
        color: var(--text-secondary);
        cursor: pointer;
        padding: 8px;
        border-radius: 8px;
        display: flex;
        align-items: center;
        justify-content: center;
        transition: background 0.2s;
    }
    .mobile-menu-btn:hover { background: rgba(255,255,255,0.1); }

    .mobile-title {
        font-size: 20px;
        font-weight: 700;
        color: white;
        margin: 0;
        flex: 1;
        background: linear-gradient(135deg, #6366f1, #a78bfa);
        -webkit-background-clip: text;
        -webkit-text-fill-color: transparent;
    }

    .mobile-network-dot {
        width: 10px;
        height: 10px;
        border-radius: 50%;
        flex-shrink: 0;
    }

    .mobile-search {
        padding: 8px 16px;
    }

    .mobile-folders-bar {
        display: flex;
        gap: 8px;
        padding: 4px 16px 12px;
        overflow-x: auto;
        scrollbar-width: none;
        -webkit-overflow-scrolling: touch;
    }
    .mobile-folders-bar::-webkit-scrollbar { display: none; }

    .mobile-folder-chip {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 6px 14px;
        border-radius: 20px;
        border: 1px solid var(--border);
        background: var(--bg-secondary, #1e1e2e);
        color: var(--text-secondary);
        font-size: 13px;
        white-space: nowrap;
        cursor: pointer;
        transition: all 0.2s;
        flex-shrink: 0;
    }
    .mobile-folder-chip.active {
        background: var(--accent, #6366f1);
        color: white;
        border-color: var(--accent, #6366f1);
        box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
    }
    .mobile-folder-chip:hover { background: rgba(255,255,255,0.1); }

    .mobile-folder-icon {
        font-size: 14px;
        display: flex;
        align-items: center;
    }
    .mobile-folder-icon :global(svg) { width: 14px; height: 14px; }

    .mobile-folder-badge {
        position: absolute;
        top: -6px;
        right: -6px;
        background: #ff4757;
        color: white;
        font-size: 9px;
        font-weight: 700;
        min-width: 16px;
        height: 16px;
        border-radius: 8px;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0 4px;
        box-shadow: 0 2px 6px rgba(255, 71, 87, 0.4);
    }

    .mobile-contacts {
        flex: 1;
        overflow-y: auto;
        -webkit-overflow-scrolling: touch;
    }

    .mobile-bottom-bar {
        display: flex;
        gap: 8px;
        padding: 10px 16px;
        padding-bottom: calc(10px + env(safe-area-inset-bottom, 0px));
        background: var(--bg-secondary, #1e1e2e);
        border-top: 1px solid var(--border);
    }

    .mobile-action-btn {
        flex: 1;
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 8px;
        padding: 12px;
        border-radius: 14px;
        border: none;
        cursor: pointer;
        font-size: 13px;
        font-weight: 600;
        transition: all 0.2s;
        color: white;
    }
    .mobile-action-btn:first-child {
        background: var(--accent, #6366f1);
    }
    .mobile-action-btn:first-child:hover {
        filter: brightness(1.1);
    }
    .mobile-action-btn:last-child {
        background: rgba(255,255,255,0.08);
        color: var(--text-primary);
    }
    .mobile-action-btn:last-child:hover {
        background: rgba(255,255,255,0.12);
    }

    .contact-header { display: flex; justify-content: space-between; align-items: center; }
    .pin-icon { width: 12px; height: 12px; margin-left: 4px; color: var(--accent); transform: rotate(45deg); display: inline-block; vertical-align: middle; }
    .pin-icon :global(svg) { width: 100%; height: 100%; }
    .contact-time { font-size: 11px; color: var(--text-secondary); opacity: 0.7; }
</style>
