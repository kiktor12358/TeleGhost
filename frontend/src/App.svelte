<script>
  import { onMount, tick } from 'svelte';
  import { fade } from 'svelte/transition';
  import { EventsOn } from '../wailsjs/runtime/runtime.js';
  import * as AppActions from '../wailsjs/go/main/App.js';
  import { writable } from 'svelte/store';
  import { Icons } from './Icons.js'; 
  import logo from './assets/images/logo.png';
  
  // Components
  import Toasts from './components/Toasts.svelte';
  import Auth from './components/Auth.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import Chat from './components/Chat.svelte';
  import Settings from './components/Settings.svelte';
  import Modals from './components/Modals.svelte';
  
  import { showToast } from './stores.js';
  import { getInitials, formatTime, parseMarkdown, getStatusColor, getStatusText } from './utils.js';

  // --- Global State ---
  let screen = 'login'; // login | main
  let identity = null;
  let isLoading = false;
  let networkStatus = 'offline';
  let myDestination = '';
  let currentUserInfo = null;
  
  // Sidebar/Contacts State
  let contacts = [];
  let searchQuery = '';
  let sidebarWidth = 300;
  let isInitializing = false;
  let selectedContact = null;
  let isResizing = false;
  let activeFolderId = 'all';
  let folders = [];
  let showAddContact = false;
  let addContactName = '';
  let addContactAddress = '';
  
  // Chat State
  let messages = [];
  let newMessage = '';
  let selectedFiles = [];
  let filePreviews = {};
  let editingMessageId = null;
  let editMessageContent = '';
  let isCompressed = true;
  let previewImage = null;
  
  // Settings State
  let showSettings = false;
  let settingsView = 'menu';
  let activeSettingsTab = 'profile';
  let profileNickname = '';
  let profileBio = '';
  let profileAvatar = '';
  let routerSettings = { tunnelLength: 1, logToFile: false };

  // Modals State
  let showConfirmModal = false;
  let confirmModalTitle = '';
  let confirmModalText = '';
  let confirmAction = null;
  
  let showSeedModal = false;
  
  let showFolderModal = false;
  let isEditingFolder = false;
  let currentFolderData = { id: '', name: '', icon: '📁' };
  
  let aboutInfo = { app_version: '', i2p_version: '', i2p_path: '', author: '', license: '' };
  
  let showContactProfile = false;
  
  // Context Menus
  let contextMenu = { show: false, x: 0, y: 0, contact: null };
  let messageContextMenu = { show: false, x: 0, y: 0, message: null };

  // Mobile View
  const mobileView = writable('list'); // 'list', 'chat', 'settings', 'search'
  let isMobile = false;

  function updateIsMobile() {
      isMobile = window.innerWidth < 768;
  }

  onMount(async () => {
    updateIsMobile();
    window.addEventListener('resize', updateIsMobile);
    
    // Check network status
    networkStatus = await AppActions.GetNetworkStatus();
    
    // Listen for events
    EventsOn("network_status", (status) => {
        networkStatus = status;
    });
    
    EventsOn("new_message", (msg) => {
        if (!msg) return;
        if (selectedContact && msg.ChatID === selectedContact.ChatID) {
            messages = [...(messages || []), msg];
            scrollToBottom();
        }
        loadContacts(); // Update last message
    });



    EventsOn("contact_updated", () => {
        console.log("[App] Received contact_updated event, reloading contacts...");
        loadContacts();
    });
  });

  async function loadMyInfo() {
      const info = await AppActions.GetMyInfo();
      if (info) {
          currentUserInfo = info;
          profileNickname = info.Nickname;
          profileAvatar = info.Avatar;
          myDestination = info.Destination;
          identity = info.ID;
      }
  }

  async function loadContacts() {
      console.log("[App] loadContacts internal started");
      try {
          console.log("[App] loadContacts: fetching contacts...");
          const result = await AppActions.GetContacts();
          console.log("[App] loadContacts: contacts received:", result?.length || 0);
          contacts = result || [];
          
          // Load folders in background without blocking contacts UI
          loadFolders();
      } catch (err) {
          console.error("[App] loadContacts failed:", err);
          throw err;
      }
      console.log("[App] loadContacts internal finished");
  }

  async function loadFolders() {
      console.log("[App] loadFolders started");
      try {
          const f = await AppActions.GetFolders();
          console.log("[App] loadFolders: folders received:", f?.length || 0);
          folders = f || [];
      } catch (err) {
          console.error("[App] loadFolders failed:", err);
      }
  }

  async function onLoginSuccess() {
      console.log("[App] onLoginSuccess started");
      if (isInitializing) {
          console.log("[App] onLoginSuccess: already initializing, skipping redundant call");
          return;
      }
      
      isInitializing = true;
      try {
          console.log("[App] onLoginSuccess: loading MyInfo...");
          await loadMyInfo();
          
          console.log("[App] onLoginSuccess: basic info loaded, switching screen early");
          screen = 'main';
          mobileView.set('list');
          isInitializing = false; // Stop overlay early

          // Load the rest in the background
          console.log("[App] onLoginSuccess: loading Contacts and Folders in background...");
          loadContacts().then(() => {
              console.log("[App] onLoginSuccess: initial background data load finished");
              loadAboutInfo();
              
              // Start background polling
              console.log("[App] Starting background contact polling...");
              setInterval(loadContacts, 300 * 1000);
          }).finally(() => {
              isInitializing = false; // Ensure overlay is removed even if loadContacts fails partially
          });
          
          console.log("[App] onLoginSuccess: transition complete!");
      } catch (err) {
          console.error("[App] onLoginSuccess failed:", err);
          showToast("Ошибка при загрузке данных: " + err, 'error');
          // Still try to show the app, maybe it's partially working
          screen = 'main';
      } finally {
          isInitializing = false;
          console.log("[App] onLoginSuccess finished, isInitializing = false");
      }
  }

  async function handleLogout() {
      console.log("[App] Logging out, resetting all states...");
      await AppActions.Logout();
      screen = 'login';
      identity = null;
      selectedContact = null;
      showSettings = false;
      contacts = [];
      folders = [];
      activeFolderId = 'all';
      searchQuery = '';
      currentUserInfo = null;
  }

  function selectContact(contact) {
      if (!contact) return;
      selectedContact = contact;
      loadMessages(contact.ID);
      if (isMobile) mobileView.set('chat');
  }

  async function loadMessages(contactId) {
      messages = await AppActions.GetMessages(contactId, 50, 0);
      scrollToBottom();
  }

  async function sendMessage() {
      if (!selectedContact || (!newMessage.trim() && selectedFiles.length === 0)) return;
      
      try {
          if (selectedFiles.length > 0) {
              await AppActions.SendFileMessage(selectedContact.id, newMessage, selectedFiles, !isCompressed);
          } else {
              await AppActions.SendText(selectedContact.id, newMessage);
          }
          newMessage = '';
          selectedFiles = [];
          filePreviews = {};
          loadMessages(selectedContact.id);
      } catch (err) {
          showToast(err, 'error');
      }
  }

  function scrollToBottom() {
      tick().then(() => {
          const container = document.querySelector('.messages-container');
          if (container) container.scrollTop = container.scrollHeight;
      });
  }

  // --- Handlers for Components ---
  const sidebarHandlers = {
      onSelectContact: selectContact,
      onContextMenu: (e, contact) => {
          contextMenu = { show: true, x: e.clientX || (e.touches ? e.touches[0].clientX : 0), y: e.clientY || (e.touches ? e.touches[0].clientY : 0), contact };
      },
      onToggleSettings: () => { showSettings = true; settingsView = 'menu'; },
      onStartResize: (e) => {
          isResizing = true;
          const handleMove = (e) => {
              if (!isResizing) return;
              let newWidth = e.clientX - 72;
              if (newWidth >= 240 && newWidth <= 600) sidebarWidth = newWidth;
          };
          const handleUp = () => {
              isResizing = false;
              window.removeEventListener('mousemove', handleMove);
              window.removeEventListener('mouseup', handleUp);
          };
          window.addEventListener('mousemove', handleMove);
          window.addEventListener('mouseup', handleUp);
      },
      onOpenAddContact: () => { 
          showAddContact = true;
          addContactName = '';
          addContactAddress = '';
      },
      onAddContactFromClipboard: async () => {
          try {
              const newContact = await AppActions.AddContactFromClipboard();
              
              // Optimistic update
              if (newContact) {
                  contacts = [newContact, ...contacts];
              }
              
              loadContacts();
              showToast("Контакт добавлен", "success");
          } catch (e) { showToast(e, "error"); }
      },
      onCopyDestination: () => {
          AppActions.CopyToClipboard(myDestination);
          showToast("Адрес скопирован", "success");
      },
      onSelectFolder: (id) => { activeFolderId = id; showSettings = false; },
      onEditFolder: (folder) => {
          isEditingFolder = true;
          currentFolderData = { ...folder };
          showFolderModal = true;
      },
      onCreateFolder: () => {
          isEditingFolder = false;
          currentFolderData = { id: '', name: '', icon: '📁' };
          showFolderModal = true;
      }
  };

  const chatHandlers = {
      onSendMessage: sendMessage,
      onKeyPress: (e) => e.key === 'Enter' && !e.shiftKey && sendMessage(),
      onPaste: async (e) => {
          const items = e.clipboardData.items;
          for (const item of items) {
              if (item.type.indexOf('image') !== -1) {
                  // Handle image paste
              }
          }
      },
      onSelectFiles: async () => {
          const files = await AppActions.SelectFiles();
          if (files) {
              selectedFiles = [...selectedFiles, ...files];
              for (const f of files) {
                  try {
                      const thumb = await AppActions.GetImageThumbnail(f);
                      if (thumb) filePreviews[f] = thumb;
                  } catch(e) {}
              }
          }
      },
      onRemoveFile: (i) => {
          selectedFiles.splice(i, 1);
          selectedFiles = [...selectedFiles];
      },
      onShowMessageMenu: (e, msg) => {
          messageContextMenu = { show: true, x: e.clientX || (e.touches ? e.touches[0].clientX : 0), y: e.clientY || (e.touches ? e.touches[0].clientY : 0), message: msg };
      },
      onAcceptTransfer: async (msg) => {
          await AppActions.AcceptFileTransfer(msg.id);
          showToast("Передача начата", "info");
      },
      onDeclineTransfer: async (msg) => {
          await AppActions.DeclineFileTransfer(msg.id);
      },
      onOpenContactProfile: () => { showContactProfile = true; },
      onSaveEditMessage: async () => {
          await AppActions.EditMessage(editingMessageId, editMessageContent);
          editingMessageId = null;
          loadMessages(selectedContact.id);
      },
      onCancelEdit: () => { editingMessageId = null; },
      onOpenFile: (path) => AppActions.OpenFile(path),
      onPreviewImage: async (path) => {
          const base64 = await AppActions.GetFileBase64(path);
          previewImage = "data:image/jpeg;base64," + base64;
      },
      startLoadingImage: (node, path) => {
          AppActions.GetFileBase64(path).then(b64 => {
              if (b64) node.src = "data:image/jpeg;base64," + b64;
          });
      }
  };

  const settingsHandlers = {
      onSaveProfile: async () => {
          await AppActions.UpdateMyProfile(profileNickname, profileBio, profileAvatar);
          showToast("Профиль сохранен", "success");
      },
      onSaveRouterSettings: async () => {
          await AppActions.SaveRouterSettings(routerSettings);
          showToast("Настройки сохранены. Требуется перезапуск.", "info");
      },
      onAvatarChange: async () => {
          try {
              const file = await AppActions.SelectImage();
              if (file) {
                  const thumb = await AppActions.GetImageThumbnail(file);
                  if (thumb) {
                      profileAvatar = "data:image/png;base64," + thumb;
                      showToast("Аватар выбран. Нажмите 'Сохранить', чтобы применить.", "info");
                  }
              }
          } catch (e) {
              showToast("Ошибка выбора файла: " + e, "error");
          }
      },
      onLogout: handleLogout,
      onTogglePinUsage: () => { /* Toggle PIN logic */ },
      onChangePin: () => { /* Change PIN logic */ },
      onBackToMenu: () => { settingsView = 'menu'; },
      onOpenCategory: (id) => { 
          activeSettingsTab = id; 
          settingsView = 'details'; 
          if (id === 'about') loadAboutInfo();
      },
      onClose: () => { showSettings = false; },
      onShowSeed: () => { showSeedModal = true; },
      onCheckUpdates: async () => {
          const res = await AppActions.CheckForUpdates();
          showToast(res, 'success');
      }
  };

  async function loadAboutInfo() {
      try {
          aboutInfo = await AppActions.GetAppAboutInfo();
      } catch (e) {
          console.error("Failed to load about info", e);
      }
  }

  const modalHandlers = {
      onConfirm: () => { confirmAction(); showConfirmModal = false; },
      onCancelConfirm: () => { showConfirmModal = false; },
      onSaveFolder: async () => {
          if (isEditingFolder) {
              await AppActions.UpdateFolder(currentFolderData.id, currentFolderData.name, currentFolderData.icon);
          } else {
              await AppActions.CreateFolder(currentFolderData.name, currentFolderData.icon);
          }
          showFolderModal = false;
          loadContacts();
      },
      onCancelFolder: () => { showFolderModal = false; },
      onCloseContactProfile: () => { showContactProfile = false; },
      onAddContact: async () => {
          console.log("Adding contact:", { name: addContactName, address: addContactAddress });
          const trimmedName = addContactName?.trim();
          const trimmedAddress = addContactAddress?.trim();

          if (!trimmedName || !trimmedAddress) {
              showToast("Заполните все поля", "error");
              return;
          }
          try {
              const newContact = await AppActions.AddContact(trimmedName, trimmedAddress);
              showAddContact = false;
              addContactName = '';
              addContactAddress = '';
              
              // Optimistic update: add to list immediately
              if (newContact) {
                  contacts = [newContact, ...contacts];
              }
              
              // Then reload to get full info (last messages etc)
              loadContacts();
              showToast("Контакт добавлен", "success");
          } catch (e) { 
              console.error("Failed to add contact:", e);
              showToast(e, "error"); 
          }
      },
      onCancelAddContact: () => { 
          showAddContact = false;
          addContactName = '';
          addContactAddress = '';
      },
      onCloseSeed: () => { showSeedModal = false; }
  };
</script>

<svelte:window on:click={() => { contextMenu.show = false; messageContextMenu.show = false; }} />

<main>
    <Toasts />

    {#if isInitializing}
        <div class="initializing-overlay animate-fade-in">
            <div class="glass-panel" style="padding: 40px; border-radius: 24px; text-align: center; border: 1px solid rgba(255,255,255,0.1); background: rgba(30,30,46,0.8); backdrop-filter: blur(20px);">
                <div class="spinner-xl"></div>
                <h2 style="margin-top: 24px; color: #fff; font-weight: 600;">Подключение...</h2>
                <p style="color: var(--text-secondary); margin-top: 8px;">Синхронизация профиля и контактов</p>
            </div>
        </div>
    {/if}

    {#if screen === 'login'}
        <Auth {logo} {onLoginSuccess} />
    {:else}
        <div class="main-screen" class:mobile-layout={isMobile}>
            {#if isMobile}
                {#if $mobileView === 'list'}
                    <Sidebar 
                        {isMobile} {contacts} {folders} {activeFolderId} {searchQuery} 
                        {networkStatus} {showSettings} {sidebarWidth} {isResizing} {selectedContact}
                        {...sidebarHandlers} 
                    />
                {:else if $mobileView === 'chat' && selectedContact}
                    <div class="content-area">
                        <Chat 
                            {selectedContact} {messages} {newMessage} {selectedFiles} {filePreviews}
                            {editingMessageId} {editMessageContent} {isCompressed} {previewImage}
                            {...chatHandlers}
                            on:back={() => mobileView.set('list')} 
                        />
                    </div>
                {:else if $mobileView === 'settings'}
                     <div class="content-area">
                        <Settings 
                            {profileNickname} {profileBio} {profileAvatar} {routerSettings} 
                            settingsCategories={[
                                {id: 'profile', name: 'Профиль', icon: Icons.User},
                                {id: 'privacy', name: 'Приватность', icon: Icons.Lock},
                                {id: 'network', name: 'I2P Сеть', icon: Icons.Globe},
                                {id: 'about', name: 'О программе', icon: Icons.Info}
                            ]}
                            {activeSettingsTab} {settingsView} selectedProfile={null} {networkStatus} {myDestination}
                            {aboutInfo}
                            {...settingsHandlers} 
                        />
                     </div>
                {:else}
                    <div class="content-area">
                        <div class="no-chat">
                            <div class="ghost-logo-wrapper">
                                <div class="icon-svg-xl">{@html Icons.Ghost}</div>
                            </div>
                            <h2>TeleGhost</h2>
                            <p>Выберите чат для начала общения</p>
                        </div>
                    </div>
                {/if}
            {:else}
                <Sidebar 
                    {isMobile} {contacts} {folders} {activeFolderId} {searchQuery} 
                    {networkStatus} {showSettings} {sidebarWidth} {isResizing} {selectedContact}
                    {...sidebarHandlers} 
                />
                
                <div class="content-area">
                    {#if showSettings}
                        <Settings 
                            {profileNickname} {profileBio} {profileAvatar} {routerSettings} 
                            settingsCategories={[
                                {id: 'profile', name: 'Профиль', icon: Icons.User},
                                {id: 'privacy', name: 'Приватность', icon: Icons.Lock},
                                {id: 'network', name: 'I2P Сеть', icon: Icons.Globe},
                                {id: 'about', name: 'О программе', icon: Icons.Info}
                            ]}
                            {activeSettingsTab} {settingsView} selectedProfile={null} {networkStatus} {myDestination}
                            {aboutInfo}
                            {...settingsHandlers} 
                        />
                    {:else if selectedContact}
                        <Chat 
                            {selectedContact} {messages} {newMessage} {selectedFiles} {filePreviews}
                            {editingMessageId} {editMessageContent} {isCompressed} {previewImage}
                            {...chatHandlers}
                        />
                    {:else}
                        <div class="no-chat animate-fade-in">
                            <div class="ghost-logo-wrapper">
                                <div class="icon-svg-xl">{@html Icons.Ghost}</div>
                            </div>
                            <h2>TeleGhost</h2>
                            <p>Выберите чат для начала общения</p>
                        </div>
                    {/if}
                </div>
            {/if}
        </div>
    {/if}

    <Modals {showConfirmModal} {confirmModalTitle} {confirmModalText} 
            {showFolderModal} {isEditingFolder} folderName={currentFolderData.name} folderIcon={currentFolderData.icon}
            showContactProfile={showContactProfile} contact={selectedContact}
            {showAddContact} bind:addContactName bind:addContactAddress
            {showSeedModal} mnemonic={currentUserInfo?.Mnemonic || ''}
            {...modalHandlers} />

    {#if previewImage}
        <div class="fullscreen-preview" on:click={() => previewImage = null}>
            <img src={previewImage} alt="Preview" />
        </div>
    {/if}

    {#if contextMenu.show}
        <div class="context-menu" style="top: {contextMenu.y}px; left: {contextMenu.x}px">
            <div class="context-item" on:click={() => { 
                AppActions.DeleteContact(contextMenu.contact.ID); 
                loadContacts();
            }}>Удалить контакт</div>
        </div>
    {/if}

    {#if messageContextMenu.show}
        <div class="context-menu" style="top: {messageContextMenu.y}px; left: {messageContextMenu.x}px">
            <div class="context-item" on:click={() => {
                editingMessageId = messageContextMenu.message.ID;
                editMessageContent = messageContextMenu.message.Content;
            }}>Редактировать</div>
            <div class="context-item danger" on:click={() => {
                AppActions.DeleteMessage(messageContextMenu.message.ID);
                loadMessages(selectedContact.ID);
            }}>Удалить</div>
        </div>
    {/if}
</main>

<style>
    :global(:root) {
        --bg-primary: #0c0c14;
        --bg-secondary: #1e1e2e;
        --bg-tertiary: #11111b;
        --bg-input: #0c0c14;
        --text-primary: #ffffff;
        --text-secondary: #a0a0ba;
        --accent: #6366f1;
        --border: rgba(255,255,255,0.05);
    }

    :global(body) {
        margin: 0;
        background: var(--bg-primary);
        color: var(--text-primary);
        font-family: 'Inter', -apple-system, sans-serif;
    }

    .main-screen { display: flex; height: 100vh; overflow: hidden; }
    .content-area { flex: 1; display: flex; flex-direction: column; position: relative; }
    
    .no-chat { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; opacity: 0.7; }

    .context-menu {
        position: fixed; background: var(--bg-secondary); border: 1px solid var(--border); border-radius: 8px; padding: 4px; z-index: 10000; box-shadow: 0 10px 30px rgba(0,0,0,0.5);
    }
    .context-item { padding: 10px 16px; cursor: pointer; border-radius: 4px; font-size: 14px; }
    .context-item:hover { background: rgba(255,255,255,0.1); }
    .context-item.danger { color: #ff6b6b; }

    .fullscreen-preview {
        position: fixed; inset: 0; background: rgba(0,0,0,0.9); z-index: 20000; display: flex; align-items: center; justify-content: center;
    }
    .fullscreen-preview img { max-width: 90%; max-height: 90%; object-fit: contain; }

    .ghost-logo-wrapper {
        width: 120px;
        height: 120px;
        background: linear-gradient(135deg, rgba(99, 102, 241, 0.1), rgba(162, 155, 254, 0.1));
        border-radius: 35px;
        display: flex;
        align-items: center;
        justify-content: center;
        color: var(--accent);
        opacity: 0.8;
        margin-bottom: 20px;
        border: 1px solid rgba(99, 102, 241, 0.1);
    }
    .icon-svg-xl { width: 64px; height: 64px; }
    .icon-svg-xl :global(svg) { width: 100%; height: 100%; }

    .initializing-overlay {
        position: fixed; inset: 0; background: var(--bg-primary); z-index: 15000;
        display: flex; align-items: center; justify-content: center;
    }
    .spinner-xl {
        width: 60px; height: 60px; border: 5px solid rgba(255,255,255,0.1);
        border-top-color: var(--accent); border-radius: 50%;
        animation: spin 1s linear infinite; margin: 0 auto;
    }
    @keyframes spin { to { transform: rotate(360deg); } }

    @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
    .animate-fade-in { animation: fadeIn 0.4s ease-out forwards; }
    
    @keyframes slideDown { from { opacity: 0; transform: translateY(-10px); } to { opacity: 1; transform: translateY(0); } }
    .animate-slide-down { animation: slideDown 0.3s ease-out forwards; }

    @keyframes messageSlide { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }
    .animate-message { animation: messageSlide 0.2s ease-out forwards; }

    /* Mobile layout specific fixes */
    .mobile-layout .sidebar { width: 100% !important; border-right: none; }
    .mobile-layout .content-area { width: 100%; height: 100%; }
</style>
