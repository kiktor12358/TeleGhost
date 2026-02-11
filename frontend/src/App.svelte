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
  let isSending = false;
  let networkStatus = 'offline';
  let myDestination = '';
  let currentUserInfo = null;
  let unreadCount = 0;
  
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
  let replyingTo = null;
  
  // Settings State
  let showSettings = false;
  let settingsView = 'menu';
  let activeSettingsTab = 'profile';
  let profileNickname = '';
  let profileBio = '';
  let profileAvatar = '';
  let routerSettings = { tunnelLength: 1, logToFile: false };
  let selectedProfile = null;

  // Modals State
  let showConfirmModal = false;
  let confirmModalTitle = '';
  let confirmModalText = '';
  let confirmAction = null;
  
  let showSeedModal = false;
  let showChangePinModal = false;
  
  let showFolderModal = false;
  let isEditingFolder = false;
  let currentFolderData = { ID: '', Name: '', Icon: '📁' };
  
  let aboutInfo = { app_version: '', i2p_version: '', i2p_path: '', author: '', license: '' };
  
  let showContactProfile = false;
  
  // Context Menus
  let contextMenu = { show: false, x: 0, y: 0, contact: null };
  let messageContextMenu = { show: false, x: 0, y: 0, message: null };
  let folderContextMenu = { show: false, x: 0, y: 0, folder: null };

  // Mobile View
  const mobileView = writable('list'); // 'list', 'chat', 'settings', 'search'
  let isMobile = false;

  function updateIsMobile() {
      isMobile = window.innerWidth < 768;
  }

  onMount(async () => {
    updateIsMobile();
    window.addEventListener('resize', updateIsMobile);
    
    // Focus tracking
    window.addEventListener('focus', () => AppActions.SetAppFocus(true));
    window.addEventListener('blur', () => AppActions.SetAppFocus(false));
    AppActions.SetAppFocus(document.hasFocus());
    
    // Back button support for mobile
    window.addEventListener('popstate', (e) => {
        if (isMobile) {
            if (showSettings) {
                showSettings = false;
                mobileView.set('list');
            } else if (selectedContact) {
                selectContact(null);
                mobileView.set('list');
            }
        }
    });
    
    // Check network status
    networkStatus = await AppActions.GetNetworkStatus();
    
    // Listen for events
    EventsOn("network_status", (status) => {
        networkStatus = status;
    });
    
    EventsOn("new_message", (msg) => {
        if (!msg) return;
        if (selectedContact && msg.ChatID === selectedContact.ChatID) {
            // Check if optimistic message exists and replace it
            const existingIdx = (messages || []).findIndex(m => m.ID === msg.ID);
            if (existingIdx !== -1) {
                const updated = [...messages];
                updated[existingIdx] = msg;
                messages = updated;
            } else {
                // Remove optimistic messages that match (by tempId prefix)
                messages = [...(messages || []).filter(m => !m._optimistic), msg];
            }
            scrollToBottom();
        }
        loadContacts(); // Update last message
    });

    EventsOn("new_contact", (data) => {
        if (data && data.nickname) {
            showToast(`Новый контакт: ${data.nickname}`, 'success', 5000);
        }
        loadContacts();
    });

    EventsOn("contact_updated", () => {
        console.log("[App] Received contact_updated event, reloading contacts...");
        loadContacts();
    });

    EventsOn("unread_count", (count) => {
        unreadCount = count;
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
      // Load current profile metadata for PIN settings
      try {
          selectedProfile = await AppActions.GetCurrentProfile();
      } catch(e) { console.warn('GetCurrentProfile failed:', e); }

      // Load unread count
      try {
          unreadCount = await AppActions.GetUnreadCount();
      } catch(e) { console.warn('GetUnreadCount failed:', e); }
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
      if (!contact) {
          selectedContact = null;
          messages = [];
          AppActions.SetActiveChat("");
          return;
      }
      if (selectedContact && selectedContact.ID === contact.ID) {
          selectedContact = null;
          messages = [];
          AppActions.SetActiveChat("");
          return;
      }
      selectedContact = contact;
      showSettings = false;
      loadMessages(contact.ID);
      AppActions.SetActiveChat(contact.ChatID || "");
      if (isMobile) {
          mobileView.set('chat');
          // Add history state for back button
          window.history.pushState({view: 'chat'}, '');
      }
  }

  async function loadMessages(contactId) {
      const contact = contacts.find(c => c.ID === contactId);
      if (contact && contact.ChatID) {
          // Помечаем чат как прочитанный
          await AppActions.MarkChatAsRead(contact.ChatID);
      }
      messages = await AppActions.GetMessages(contactId, 50, 0);
      scrollToBottom();
  }

  async function sendMessage() {
      if (!selectedContact || (!newMessage.trim() && selectedFiles.length === 0)) return;
      if (isSending) return;
      
      // Client-side length check
      if (newMessage.length > 4096) {
          showToast(`Сообщение слишком длинное (${newMessage.length}/4096)`, 'error');
          return;
      }
      
      isSending = true;
      const text = newMessage;
      const files = [...selectedFiles];
      const compress = isCompressed;
      
      // Optimistic UI — мгновенно показываем сообщение
      const tempId = '_opt_' + Date.now().toString();
      const optimisticMsg = {
          ID: tempId,
          Content: text,
          Timestamp: Date.now(),
          IsOutgoing: true,
          Status: 'sending',
          ContentType: files.length > 0 ? 'mixed' : 'text',
          ReplyToID: replyingTo?.ID,
          ReplyPreview: replyingTo ? { AuthorName: replyingTo.SenderID === identity ? 'Я' : selectedContact.Nickname, Content: replyingTo.Content } : null,
          _optimistic: true
      };
      
      const replyID = replyingTo?.ID || "";
      replyingTo = null; // Clear immediately after getting ID
      
      messages = [...(messages || []), optimisticMsg];
      scrollToBottom();
      
      // Очищаем инпут мгновенно
      newMessage = '';
      selectedFiles = [];
      filePreviews = {};
      
      try {
          if (files.length > 0) {
              await AppActions.SendFileMessage(selectedContact.ID, text, replyID, files, !compress);
          } else {
              await AppActions.SendText(selectedContact.ID, text, replyID);
          }
          // Убираем оптимистичное сообщение (реальное придёт через событие)
          messages = (messages || []).filter(m => m.ID !== tempId);
          await loadMessages(selectedContact.ID);
      } catch (err) {
          showToast(err, 'error');
          // Помечаем как ошибку
          messages = (messages || []).map(m => m.ID === tempId ? {...m, Status: 'failed', _optimistic: false} : m);
      } finally {
          isSending = false;
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
      onToggleSettings: () => { 
          if (showSettings) {
              showSettings = false;
              if (isMobile) mobileView.set('list');
          } else {
              showSettings = true;
              settingsView = 'menu';
              if (isMobile) {
                  mobileView.set('settings');
                  window.history.pushState({view: 'settings'}, '');
              }
          }
      },
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
          currentFolderData = { 
              ID: folder.ID || folder.id, 
              Name: folder.Name || folder.name, 
              Icon: folder.Icon || folder.icon 
          };
          showFolderModal = true;
      },
      onCreateFolder: () => {
          isEditingFolder = false;
          currentFolderData = { ID: '', Name: '', Icon: '📁' };
          showFolderModal = true;
      },
      onFolderContextMenu: (e, folder) => {
          folderContextMenu = { show: true, x: e.clientX, y: e.clientY, folder: folder };
      }
  };

  const chatHandlers = {
      onSendMessage: sendMessage,
      onKeyPress: (e) => e.key === 'Enter' && !e.shiftKey && sendMessage(),
      onPaste: async (e) => {
          const items = (e.clipboardData || e.originalEvent?.clipboardData)?.items;
          if (!items) return;
          for (let i = 0; i < items.length; i++) {
              const item = items[i];
              if (item.kind === 'file' && item.type.indexOf('image/') !== -1) {
                  e.preventDefault();
                  const blob = item.getAsFile();
                  const reader = new FileReader();
                  reader.onload = async (event) => {
                      try {
                          const base64 = event.target.result;
                          const path = await AppActions.SaveTempImage(base64, 'pasted_image.png');
                          if (selectedFiles.length < 6) {
                              selectedFiles = [...selectedFiles, path];
                              const previewB64 = base64.split(',')[1];
                              filePreviews[path] = previewB64;
                              filePreviews = filePreviews;
                          } else {
                              showToast('Максимум 6 вложений', 'error');
                          }
                      } catch (err) {
                          console.error('Paste error', err);
                          showToast('Ошибка вставки изображения', 'error');
                      }
                  };
                  reader.readAsDataURL(blob);
                  return;
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
          let x = e.clientX || (e.touches ? e.touches[0].clientX : 0);
          let y = e.clientY || (e.touches ? e.touches[0].clientY : 0);
          // Prevent overflow
          const menuWidth = 200;
          const menuHeight = 180;
          if (x + menuWidth > window.innerWidth) x = window.innerWidth - menuWidth - 10;
          if (y + menuHeight > window.innerHeight) y = window.innerHeight - menuHeight - 10;
          messageContextMenu = { show: true, x, y, message: msg };
      },
      onAcceptTransfer: async (msg) => {
          await AppActions.AcceptFileTransfer(msg.ID);
          showToast("Передача начата", "info");
      },
      onDeclineTransfer: async (msg) => {
          await AppActions.DeclineFileTransfer(msg.ID);
      },
      onOpenContactProfile: () => { showContactProfile = true; },
      onSaveEditMessage: async () => {
          await AppActions.EditMessage(editingMessageId, editMessageContent);
          editingMessageId = null;
          loadMessages(selectedContact.ID);
      },
      onCancelEdit: () => { editingMessageId = null; },
      onOpenFile: (path) => AppActions.OpenFile(path),
      onSaveFile: async (path, filename) => {
          try {
              await AppActions.SaveFileToLocation(path, filename);
              showToast("Файл сохранен", "success");
          } catch (e) {
              if (e) showToast(e, "error");
          }
      },
      onPreviewImage: async (path) => {
          const base64 = await AppActions.GetFileBase64(path);
          previewImage = "data:image/jpeg;base64," + base64;
      },
      startLoadingImage: (node, path) => {
          AppActions.GetFileBase64(path).then(b64 => {
              if (b64) node.src = "data:image/jpeg;base64," + b64;
          });
      },
      onCancelReply: () => { replyingTo = null; }
  };

  const settingsHandlers = {
      onSaveProfile: async () => {
          await AppActions.UpdateMyProfile(profileNickname, profileBio, profileAvatar);
          await loadMyInfo(); // Refresh state
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
      onTogglePinUsage: async () => {
          if (!selectedProfile) return;
          try {
              const newUsePin = !selectedProfile.use_pin;
              if (newUsePin) {
                  // Включаем PIN — нужен новый PIN
                  const pin = prompt('Введите новый ПИН-код (минимум 6 символов):');
                  if (!pin || pin.length < 6) { showToast('ПИН-код должен быть минимум 6 символов', 'error'); return; }
                  const mnemonic = currentUserInfo?.Mnemonic || '';
                  await AppActions.UpdateProfile(selectedProfile.id, '', '', false, true, pin, mnemonic);
              } else {
                  // Отключаем PIN
                  const mnemonic = currentUserInfo?.Mnemonic || '';
                  await AppActions.UpdateProfile(selectedProfile.id, '', '', false, false, '', mnemonic);
              }
              selectedProfile = await AppActions.GetCurrentProfile();
              showToast(newUsePin ? 'ПИН-код включён' : 'ПИН-код отключён', 'success');
          } catch(e) { showToast('Ошибка: ' + e, 'error'); }
      },
      onChangePin: async () => {
          if (!selectedProfile) return;
          showChangePinModal = true;
      },
      onBackToMenu: () => { settingsView = 'menu'; },
      onOpenCategory: (id) => { 
          activeSettingsTab = id; 
          settingsView = 'details'; 
          if (id === 'about') loadAboutInfo();
      },
      onClose: () => { 
          showSettings = false; 
          if (isMobile) mobileView.set('list');
      },
      onUpdateProfile: async (addr) => {
          try {
              await AppActions.RequestProfileUpdate(addr);
              showToast("Запрос на обновление профиля отправлен", "info");
          } catch (e) { showToast(e, "error"); }
      },
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
              await AppActions.UpdateFolder(currentFolderData.ID, currentFolderData.Name, currentFolderData.Icon);
          } else {
              await AppActions.CreateFolder(currentFolderData.Name, currentFolderData.Icon);
          }
          showFolderModal = false;
          loadContacts();
      },
      onDeleteFolder: async () => {
          const folder = folderContextMenu.folder || (isEditingFolder ? currentFolderData : null);
          const folderID = folder?.ID || folder?.id;
          if (!folderID) return;
          
          showConfirmModal = true;
          confirmModalTitle = "Удалить папку";
          confirmModalText = `Вы уверены, что хотите удалить папку "${folder.Name || folder.name}"? Сами чаты останутся в общем списке.`;
          confirmAction = async () => {
              await AppActions.DeleteFolder(folderID);
              showFolderModal = false;
              folderContextMenu.show = false;
              loadContacts();
              showToast("Папка удалена", "success");
          };
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
      onCloseSeed: () => { showSeedModal = false; },
      onSavePin: async (pin) => {
          if (!selectedProfile) return;
          try {
              const mnemonic = currentUserInfo?.Mnemonic || '';
              await AppActions.UpdateProfile(selectedProfile.id, '', '', false, true, pin, mnemonic);
              selectedProfile = await AppActions.GetCurrentProfile();
              showChangePinModal = false;
              showToast('ПИН-код обновлён', 'success');
          } catch(e) { 
              showToast('Ошибка: ' + e, 'error'); 
          }
      },
      onCancelChangePin: () => { showChangePinModal = false; }
  };
</script>

<svelte:window 
    on:click={() => { contextMenu.show = false; messageContextMenu.show = false; folderContextMenu.show = false; }} 
    on:keydown={(e) => {
        if (e.key === 'Escape') {
            messageContextMenu.show = false;
            contextMenu.show = false;
            folderContextMenu.show = false;
            if (editingMessageId) { editingMessageId = null; editMessageContent = ''; }
            if (previewImage) { previewImage = null; }
            if (showSettings && isMobile) { showSettings = false; mobileView.set('list'); }
            if (showAddContact) { showAddContact = false; }
            if (showContactProfile) { showContactProfile = false; }
        }
    }}
/>

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
                        {unreadCount}
                        {...sidebarHandlers} 
                    />
                {:else if $mobileView === 'chat' && selectedContact}
                    <div class="content-area">
                        <Chat 
                            {selectedContact} {messages} bind:newMessage bind:selectedFiles {filePreviews}
                            {editingMessageId} {editMessageContent} bind:isCompressed {previewImage}
                            bind:replyingTo {isMobile}
                            onBack={() => { selectContact(null); mobileView.set('list'); }}
                            {...chatHandlers}
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
                            {activeSettingsTab} {settingsView} {selectedProfile} {networkStatus} {myDestination}
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
                    {unreadCount}
                    {...sidebarHandlers} 
                />
                
                <div class="content-area">
                    {#if showSettings}
                        <Settings 
                            bind:profileNickname={profileNickname} 
                            bind:profileBio={profileBio} 
                            bind:profileAvatar={profileAvatar} 
                            {routerSettings} 
                            settingsCategories={[
                                {id: 'profile', name: 'Профиль', icon: Icons.User},
                                {id: 'privacy', name: 'Приватность', icon: Icons.Lock},
                                {id: 'network', name: 'I2P Сеть', icon: Icons.Globe},
                                {id: 'about', name: 'О программе', icon: Icons.Info}
                            ]}
                            {activeSettingsTab} {settingsView} {selectedProfile} {networkStatus} {myDestination}
                            {aboutInfo}
                            {...settingsHandlers} 
                        />
                    {:else if selectedContact}
                        <Chat 
                            {selectedContact} {messages} bind:newMessage bind:selectedFiles {filePreviews}
                            {editingMessageId} {editMessageContent} bind:isCompressed {previewImage}
                            bind:replyingTo isMobile={false}
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

    <Modals
        {showConfirmModal} {confirmModalTitle} {confirmModalText} {onConfirm} onCancelConfirm={() => showConfirmModal = false}
        {showFolderModal} {isEditingFolder} bind:folderName={currentFolderData.Name} bind:folderIcon={currentFolderData.Icon}
        showContactProfile={showContactProfile} contact={selectedContact} onCloseContactProfile={() => showContactProfile = false} onUpdateProfile={settingsHandlers.onUpdateProfile}
        {showAddContact} onAddContact={contactHandlers.onAddContact} onCancelAddContact={contactHandlers.onCancelAddContact} bind:addContactName bind:addContactAddress
        {showSeedModal} mnemonic={currentUserInfo?.Mnemonic || ''} onCloseSeed={contactHandlers.onCloseSeed}
        {showChangePinModal} onSavePin={contactHandlers.onSavePin} onCancelChangePin={contactHandlers.onCancelChangePin}
    />

    {#if previewImage}
        <div class="fullscreen-preview" on:click={() => previewImage = null}>
            <img src={previewImage} alt="Preview" />
        </div>
    {/if}

    {#if contextMenu.show}
        <div class="context-menu" style="top: {contextMenu.y}px; left: {contextMenu.x}px">
            {#if folders.length > 0}
                {@const inFolders = folders.filter(f => (f.ChatIDs || f.chat_ids || []).includes(contextMenu.contact.ID))}
                {@const notInFolders = folders.filter(f => !(f.ChatIDs || f.chat_ids || []).includes(contextMenu.contact.ID))}

                {#if notInFolders.length > 0}
                    <div class="context-item submenu-parent">
                        Добавить в папку
                        <div class="context-submenu">
                            {#each notInFolders as folder}
                                <div class="context-item" on:click={async () => {
                                    await AppActions.AddChatToFolder(folder.ID || folder.id, contextMenu.contact.ID);
                                    loadFolders();
                                    contextMenu.show = false;
                                    showToast(`Добавлено в папку "${folder.Name || folder.name}"`, 'success');
                                }}>{folder.Icon || folder.icon} {folder.Name || folder.name}</div>
                            {/each}
                        </div>
                    </div>
                {/if}

                {#if inFolders.length > 0}
                    <div class="context-item submenu-parent">
                        Удалить из папки
                        <div class="context-submenu">
                            {#each inFolders as folder}
                                <div class="context-item" on:click={async () => {
                                    await AppActions.RemoveChatFromFolder(folder.ID || folder.id, contextMenu.contact.ID);
                                    loadFolders();
                                    contextMenu.show = false;
                                    showToast(`Удалено из папки "${folder.Name || folder.name}"`, 'success');
                                }}>{folder.Icon || folder.icon} {folder.Name || folder.name}</div>
                            {/each}
                        </div>
                    </div>
                {/if}
            {/if}
            <div class="context-item danger" on:click={() => { 
                AppActions.DeleteContact(contextMenu.contact.ID); 
                loadContacts();
            }}>Удалить контакт</div>
        </div>
    {/if}

    {#if folderContextMenu.show}
        <div class="context-menu" style="top: {folderContextMenu.y}px; left: {folderContextMenu.x}px">
            <div class="context-item" on:click={() => { 
                sidebarHandlers.onEditFolder(folderContextMenu.folder);
                folderContextMenu.show = false;
            }}>Редактировать</div>
            <div class="context-item danger" on:click={() => {
                modalHandlers.onDeleteFolder();
            }}>Удалить папку</div>
        </div>
    {/if}

    {#if messageContextMenu.show}
        <div class="context-menu" style="top: {messageContextMenu.y}px; left: {messageContextMenu.x}px">
            <div class="context-item" on:click={() => {
                replyingTo = messageContextMenu.message;
                messageContextMenu.show = false;
                // Focus textarea
                setTimeout(() => {
                    const ta = document.querySelector('.message-input');
                    if (ta) ta.focus();
                }, 100);
            }}>Ответить</div>
            {#if messageContextMenu.message?.Content}
                <div class="context-item" on:click={() => {
                    AppActions.CopyToClipboard(messageContextMenu.message.Content);
                    showToast('Текст скопирован', 'success');
                    messageContextMenu.show = false;
                }}>Копировать текст</div>
            {/if}
            {#if messageContextMenu.message?.IsOutgoing}
                <div class="context-item" on:click={() => {
                    editingMessageId = messageContextMenu.message.ID;
                    editMessageContent = messageContextMenu.message.Content;
                    messageContextMenu.show = false;
                }}>Редактировать</div>
            {/if}
            <div class="context-item danger" on:click={() => {
                AppActions.DeleteMessage(messageContextMenu.message.ID);
                loadMessages(selectedContact.ID);
                messageContextMenu.show = false;
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

    .main-screen { display: flex; height: 100dvh; overflow: hidden; }
    .content-area { flex: 1; display: flex; flex-direction: column; position: relative; }
    
    .no-chat { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; opacity: 0.7; }

    .context-menu {
        position: fixed; background: var(--bg-secondary); border: 1px solid var(--border); border-radius: 8px; padding: 4px; z-index: 10000; box-shadow: 0 10px 30px rgba(0,0,0,0.5);
    }
    .context-item { padding: 10px 16px; cursor: pointer; border-radius: 4px; font-size: 14px; position: relative; }
    .context-item:hover { background: rgba(255,255,255,0.1); }
    .context-item.danger { color: #ff6b6b; }

    .submenu-parent {
        position: relative;
    }
    .submenu-parent:hover .context-submenu {
        display: block;
    }
    .context-submenu {
        display: none;
        position: absolute;
        left: 100%;
        top: 0;
        background: var(--bg-secondary);
        border: 1px solid var(--border);
        border-radius: 8px;
        padding: 4px;
        min-width: 150px;
        box-shadow: 0 10px 30px rgba(0,0,0,0.5);
        margin-left: 4px;
    }

    .btn-danger {
        background: #ff4757;
        color: white;
        border: none;
    }
    .btn-danger:hover {
        background: #ff6b81;
        transform: translateY(-1px);
        box-shadow: 0 4px 12px rgba(255, 71, 87, 0.3);
    }

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
