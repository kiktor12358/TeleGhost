<script>
  import { onMount, tick } from 'svelte';
  import { EventsOn } from '../wailsjs/runtime/runtime.js';
  import { 
    Login, 
    CreateAccount,
    GetNetworkStatus,
    GetMyDestination,
    GetMyInfo,
    GetContacts,
    GetMessages,
    SendText,
    AddContactFromClipboard,
    AddContact,
    DeleteContact,
    CopyToClipboard,
    UpdateMyProfile,
    EditMessage,
    DeleteMessage,
    DeleteMessageForAll,
    GetFolders,
    CreateFolder,
    DeleteFolder,
    UpdateFolder,
    AddChatToFolder,
    RemoveChatFromFolder,
    SelectFiles,
    SendFileMessage,
    GetFileBase64,
    CopyImageToClipboard,
    GetImageThumbnail,
    AcceptFileTransfer,
    DeclineFileTransfer,
    DeclineFileTransfer,
    RequestProfileUpdate,
    OpenFile,
    ShowInFolder
  } from '../wailsjs/go/main/App.js';
  import logo from './assets/images/logo.png';

  // Состояние
  let isLoading = false;
  let seedPhrase = '';
  let networkStatus = 'offline';
  let myDestination = '';
  let wailsReady = false;
  let myInfo = null;
  
  // Контакты и сообщения
  let contacts = [];
  let selectedContact = null;
  let messages = [];
  let newMessage = '';
  
  // Вложения
  let selectedFiles = []; // paths
  let filePreviews = {}; // path -> base64
  let isCompressed = true;
  let isSending = false;

  // Context Menu для контактов
  let contextMenu = { show: false, x: 0, y: 0, contact: null };

  // Context Menu для сообщений
  // Added filePath for file context menu
  let messageContextMenu = { show: false, x: 0, y: 0, message: null, imagePath: null, filePath: null };
  
  // Редактирование сообщений
  let editingMessageId = null;
  let editMessageContent = '';

  // Profile Modal
  let showContactProfile = false;
  let profileContact = null;

  // Экраны
  let screen = 'login'; // login | main | settings
  let showSettings = false;
  let showAddContact = false;
  
  // Настройки профиля
  let profileNickname = 'User';
  let profileAvatar = '';
  let avatarFileInput; // Ref for file input
  let profileBio = '';
  let addContactName = '';
  let addContactAddress = '';
  let searchQuery = '';

  // UI State: Folders & Resize
  let sidebarWidth = 300;
  let isResizing = false;
  
  // Folders State
  let folders = [];
  let showCreateFolder = false;
  let newFolderName = '';
  let newFolderIcon = '📁';
  let folderIcons = ['📁', '💼', '🏠', '🎓', '🎮', '❤️', '⭐', '🔥'];

  // Reactive folders list for UI
  $: uiFolders = [
    { id: 'all', name: 'Все', icon: '💬' },
    ...folders.sort((a, b) => a.position - b.position),
    { id: 'add', name: 'Папка', icon: '➕' }
  ];

  let activeFolderId = 'all';

  // --- UI Imprv Phase 2 ---
  // Image Preview
  let previewImage = null; // src if valid

  // Confirmation Modal
  let showConfirmModal = false;
  let confirmModalTitle = '';
  let confirmModalText = '';
  let confirmModalCallback = null;

  function openConfirmModal(title, text, callback) {
      confirmModalTitle = title;
      confirmModalText = text;
      confirmModalCallback = callback;
      showConfirmModal = true;
  }

  function closeConfirmModal() {
      showConfirmModal = false;
      confirmModalCallback = null;
  }

  function handleConfirmAction() {
      if (confirmModalCallback) confirmModalCallback();
      closeConfirmModal();
  }

  // Folders Edit State
  let isEditingFolder = false;
  let editingFolderId = null;
  let showDeleteFolderConfirm = false;
  let folderToDelete = null;

  // Filter contacts by folder AND search query
  $: filteredContacts = contacts.filter(c => {
    // Filter by folder
    if (activeFolderId !== 'all') {
      const folder = folders.find(f => f.id === activeFolderId);
      if (!folder || !folder.chatIds || !folder.chatIds.includes(c.id)) {
        return false;
      }
    }
    // Filter by search query
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return c.nickname.toLowerCase().includes(q) || 
             (c.lastMessage && c.lastMessage.toLowerCase().includes(q));
    }
    return true;
  });
  
  let settingsCategories = [
    { id: 'profile', name: 'Аккаунт', icon: '👤' },
    { id: 'chats', name: 'Настройки чатов', icon: '💬' },
    { id: 'privacy', name: 'Конфиденциальность', icon: '🔒' },
    { id: 'network', name: 'Настройки I2P/Сети', icon: '🌐' },
    { id: 'about', name: 'О программе', icon: 'ℹ️' }
  ];
  let activeSettingsTab = 'profile';

  let routerSettings = {
    tunnelLength: 1,
    logToFile: false
  };

  async function loadRouterSettings() {
    try {
      // @ts-ignore
      const settings = await window.go.main.App.GetRouterSettings();
      if (settings) {
        routerSettings = settings;
      }
    } catch (e) {
      console.error("Failed to load router settings:", e);
    }
  }

  async function saveRouterSettings() {
    try {
      // @ts-ignore
      await window.go.main.App.SaveRouterSettings(routerSettings);
      showToast("Настройки роутера сохранены. Перезапустите приложение.", "success");
    } catch (e) {
      console.error("Failed to save router settings:", e);
      showToast("Ошибка при сохранении настроек: " + e, "error");
    }
  }

  // Load settings when opening network tab
  $: if (showSettings && activeSettingsTab === 'network') {
    loadRouterSettings();
  }

  function startResize(e) {
    isResizing = true;
    document.addEventListener('mousemove', handleResize);
    document.addEventListener('mouseup', stopResize);
    document.body.style.userSelect = 'none';
    document.body.style.cursor = 'col-resize';
  }

  function handleResize(e) {
    if (!isResizing) return;
    // 72px is width of folders rail
    let newWidth = e.clientX - 72;
    if (newWidth < 240) newWidth = 240;
    if (newWidth > 600) newWidth = 600;
    sidebarWidth = newWidth;
  }

  function stopResize() {
    isResizing = false;
    document.removeEventListener('mousemove', handleResize);
    document.removeEventListener('mouseup', stopResize);
    document.body.style.userSelect = '';
    document.body.style.cursor = '';
  }

  // Toast уведомления
  let toasts = [];
  let toastId = 0;

  function showToast(message, type = 'info', duration = 3000) {
    const id = ++toastId;
    toasts = [...toasts, { id, message, type }];
    setTimeout(() => {
      toasts = toasts.filter(t => t.id !== id);
    }, duration);
  }

  // Простой markdown парсер
  function parseMarkdown(text) {
    if (!text) return '';
    
    // Экранируем HTML
    let html = text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
    
    // Код блок ```code```
    html = html.replace(/```([\s\S]*?)```/g, '<pre class="md-code-block">$1</pre>');
    
    // Инлайн код `code`
    html = html.replace(/`([^`]+)`/g, '<code class="md-code">$1</code>');
    
    // Жирный **text** или __text__
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/__([^_]+)__/g, '<strong>$1</strong>');
    
    // Курсив *text* или _text_ (но не внутри слов)
    html = html.replace(/(?<!\w)\*([^*]+)\*(?!\w)/g, '<em>$1</em>');
    html = html.replace(/(?<!\w)_([^_]+)_(?!\w)/g, '<em>$1</em>');
    
    // Зачёркнутый ~~text~~
    html = html.replace(/~~([^~]+)~~/g, '<del>$1</del>');
    
    // Ссылки [text](url)
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
    
    return html;
  }

  // Проверка готовности Wails
  function checkWailsReady() {
    return typeof window !== 'undefined' && 
           typeof window.go !== 'undefined' && 
           typeof window.go.main !== 'undefined';
  }

  async function waitForWails(maxAttempts = 50) {
    for (let i = 0; i < maxAttempts; i++) {
      if (checkWailsReady()) {
        wailsReady = true;
        return true;
      }
      await new Promise(r => setTimeout(r, 100));
    }
    return false;
  }

  onMount(async () => {
    // Close context menus on click elsewhere
    document.addEventListener('click', () => {
      contextMenu.show = false;
      messageContextMenu.show = false;
      if (editingMessageId) {
        editingMessageId = null;
        editMessageContent = '';
      }
    });

    await waitForWails();
    if (!wailsReady) return;

    try {
      EventsOn('network_status', (status) => {
        networkStatus = status;
      });

      EventsOn('new_message', (msg) => {
        console.log('[App] New message received:', msg);
        if(selectedContact) console.log('[App] Selected:', selectedContact.chatId, 'MsgChat:', msg.chatId, 'Sender:', msg.senderId);

        // Проверяем совпадение по chatId или по senderId контакта
        if (selectedContact && (msg.chatId === selectedContact.chatId || msg.senderId === selectedContact.publicKey)) {
          // Обновляем или добавляем сообщение
          const existingIdx = messages.findIndex(m => m.id === msg.id);
          if (existingIdx !== -1) {
              const updatedMessages = [...messages];
              updatedMessages[existingIdx] = msg;
              messages = updatedMessages;
          } else {
              messages = [...messages, msg];
              scrollToBottom();
          }
        }
        // Обновляем список контактов (для отображения последнего сообщения)
        loadContacts();
      });

      // Обработка входящего запроса дружбы (handshake)
      EventsOn('new_contact', (data) => {
        showToast(`📩 Новый контакт: ${data.nickname}`, 'success', 5000);
        loadContacts();
      });

      const status = await GetNetworkStatus();
      if (status) networkStatus = status;
    } catch (e) {
      console.error('Init error:', e);
    }
  });

  // === Auth ===
  async function handleLogin() {
    if (!seedPhrase.trim()) return;
    if (!wailsReady) await waitForWails();
    if (!wailsReady) { showToast('Приложение не готово', 'error'); return; }
    
    isLoading = true;
    try {
      await Login(seedPhrase);
      screen = 'main';
      await loadInitialData();
    } catch (e) {
      showToast('Ошибка: ' + e, 'error');
    }
    isLoading = false;
  }

  // Mnemonic Modal State
  let showMnemonicModal = false;
  let newMnemonic = '';

  async function handleCreateAccount() {
    if (!wailsReady) await waitForWails();
    if (!wailsReady) { showToast('Приложение не готово', 'error'); return; }
    
    isLoading = true;
    try {
      const mnemonic = await CreateAccount();
      if (mnemonic) {
        newMnemonic = mnemonic;
        showMnemonicModal = true;
      }
    } catch (e) {
      showToast('Ошибка: ' + e, 'error');
    }
    isLoading = false;
  }

  function confirmMnemonicSaved() {
    showMnemonicModal = false;
    seedPhrase = newMnemonic;
    screen = 'main';
    loadInitialData();
  }

  async function loadInitialData() {
    try {
      myDestination = await GetMyDestination() || '';
      myInfo = await GetMyInfo();
      if (myInfo) {
        profileNickname = myInfo.nickname || 'User';
        profileAvatar = myInfo.avatar || '';
      }
      await loadContacts();
      await loadFolders(); // Load folders here too!
    } catch (e) {
      console.error('Load error:', e);
    }
  }

  async function loadContacts() {
    try {
      contacts = await GetContacts() || [];
    } catch (e) {
      console.error('Contacts error:', e);
    }
  }

  // === Contacts ===
  async function selectContact(contact) {
    selectedContact = contact;
    showSettings = false;
    await loadMessages();
  }

  function handleContextMenu(e, contact) {
    e.preventDefault();
    contextMenu = {
      show: true,
      x: e.clientX,
      y: e.clientY,
      contact: contact
    };
  }

  async function deleteContactFromMenu() {
    if (!contextMenu.contact) return;
    openConfirmModal(
        `Удалить контакт ${contextMenu.contact.nickname}?`,
        "История переписки будет сохранена, но контакт исчезнет из списка.",
        async () => {
             try {
                await DeleteContact(contextMenu.contact.id);
                if (selectedContact?.id === contextMenu.contact.id) {
                    selectedContact = null;
                }
                await loadContacts();
            } catch(e) {
                showToast('Ошибка удаления: ' + e, 'error');
            }
        }
    );
    contextMenu.show = false;
  }

  async function copyAddressFromMenu() {
      if(!contextMenu.contact) return;
      // We need full address, but we might only have short info in contact list?
      // Actually backend returns full address for copy? No, ContactInfo struct has "I2PAddress" which is truncated?
      // Wait, GetContacts truncates it?
      // Backend: shortAddr := c.I2PAddress... if len > 32 ...
      // Ah, GetContacts returns truncated address. That's bad for copying.
      // But we can't fix backend easily now without modifying GetContacts structs.
      // Actually, wait. The user wants to "manage contacts".
      // Let's assume for now we just delete.
      // Or we can fix GetContacts to return full address in a separate field?
      // I'll stick to delete for now.
  }
  
  function openContactProfile() {
      if (!selectedContact) return;
      profileContact = selectedContact; // Note: this contact object has truncated address!
      showContactProfile = true;
  }

  async function loadMessages() {
    if (!selectedContact) return;
    try {
      messages = await GetMessages(selectedContact.id, 100, 0) || [];
      messages = messages.reverse();
      setTimeout(scrollToBottom, 100);
    } catch (e) {
      console.error('Messages error:', e);
    }
  }

  async function handleAddContactFromClipboard() {
    try {
      const contact = await AddContactFromClipboard();
      if (contact) {
        await loadContacts();
        showToast('Контакт добавлен: ' + contact.nickname, 'success');
      }
    } catch (e) {
      showToast(e.toString(), 'error');
    }
  }

  async function handleAddContactManual() {
    if (!addContactName.trim() || !addContactAddress.trim()) return;
    
    // Basic frontend validation
    const addr = addContactAddress.trim();
    if (addr.length < 50) {
         showToast('Слишком короткий I2P адрес (должен быть > 50 символов)', 'error');
         return;
    }

    try {
      await AddContact(addContactName, addr);
      await loadContacts();
      showAddContact = false;
      addContactName = '';
      addContactAddress = '';
      showToast('Контакт добавлен', 'success');
    } catch (e) {
      showToast(e.toString(), 'error');
    }
  }

  // === Messages ===
  
  async function handleSelectFiles() {
      try {
          const files = await SelectFiles();
          if (!files || files.length === 0) return;

          const availableSlots = 6 - selectedFiles.length;
          if (availableSlots <= 0) {
              showToast('Максимум 6 вложений', 'error');
              return;
          }
          
          const newFiles = files.slice(0, availableSlots);
          // Immediately add paths so user sees loading state
          selectedFiles = [...selectedFiles, ...newFiles];
          
          // Load thumbnails in background without blocking state too much
          for (const path of newFiles) {
              if (!filePreviews[path]) {
                  const ext = path.split('.').pop().toLowerCase();
                  if (['jpg', 'jpeg', 'png', 'webp', 'gif', 'bmp'].includes(ext)) {
                      GetImageThumbnail(path, 100, 100).then(b64 => {
                          filePreviews[path] = b64;
                          filePreviews = filePreviews; // Trigger reactivity
                      }).catch(e => console.error("Thumb error", e));
                  } else {
                      // For non-image files, just store a placeholder or null
                      filePreviews[path] = null; // Or a specific icon base64
                      filePreviews = filePreviews; // Trigger reactivity
                  }
              }
          }
      } catch (e) {
          console.error(e);
          showToast('Ошибка выбора файлов: ' + e, 'error');
      }
  }

  function removeFile(index) {
    const fileToRemove = selectedFiles[index];
    selectedFiles = selectedFiles.filter((_, i) => i !== index);
    // Remove preview for the removed file
    const newFilePreviews = { ...filePreviews };
    delete newFilePreviews[fileToRemove];
    filePreviews = newFilePreviews;
  }

  // Action for loading images in messages
  function startLoadingImage(node, path) {
      GetFileBase64(path).then(b64 => {
          node.src = "data:image/jpeg;base64," + b64;
      }).catch(e => {
          console.error("Failed to load image", path, e);
      });
  }

  async function sendMessage() {
    if ((!newMessage.trim() && selectedFiles.length === 0) || !selectedContact) return;
    if (isSending) return;
    
    // Client-side length check
    if (newMessage.length > 4096) {
        showToast(`Сообщение слишком длинное (${newMessage.length}/4096)`, 'error');
        return;
    }
    
    isSending = true;
    const text = newMessage;
    const files = [...selectedFiles];
    const compress = isCompressed; // capture state
    
    // Optimistic UI update? 
    // Hard with images because we need to display them immediately.
    // We can use filePreviews for optimistic rendering.
    
    const tempId = Date.now().toString();
    const optimisticMsg = {
      id: tempId,
      content: text,
      timestamp: Date.now(),
      isOutgoing: true,
      status: 'sending',
      attachments: files.map(path => ({
          local_path: path,
          // temporary preview logic handled by same action if path is local
      }))
    };

    messages = [...messages, optimisticMsg];
    scrollToBottom();
    
    // Clear input immediately
    newMessage = '';
    selectedFiles = [];
    filePreviews = {}; // Clear all previews
    
    try {
        if (files.length > 0) {
            await SendFileMessage(selectedContact.id, text, files, !compress);
        } else {
            await SendText(selectedContact.id, text);
        }
        // Успех - удаляем оптимистичное сообщение, так как должно прийти реальное событие
        messages = messages.filter(m => m.id !== tempId);
    } catch (e) {
      showToast(e.toString(), 'error');
      // Mark failed
      messages = messages.map(m => m.id === tempId ? {...m, status: 'failed'} : m);
    } finally {
        isSending = false;
    }
  }

  function handleKeyPress(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  }

  // === Settings ===
  async function saveProfile() {
    try {
      await UpdateMyProfile(profileNickname, profileBio, profileAvatar);
      showToast('Профиль сохранён', 'success');
    } catch (e) {
      showToast(e.toString(), 'error');
    }
  }

  function copyDestination() {
    CopyToClipboard(myDestination);
    showToast('Скопировано!', 'success');
  }

  function scrollToBottom() {
    requestAnimationFrame(() => {
      const chat = document.querySelector('.messages-container');
      if (chat) {
        chat.scrollTop = chat.scrollHeight;
      }
    });
  }

  async function requestProfileUpdate(contactID) {
    try {
      showToast('Запрос обновления профиля отправлен...', 'info');
      await RequestProfileUpdate(contactID);
    } catch (err) {
      console.error('Failed to request profile update:', err);
      showToast('Ошибка запроса обновления: ' + err, 'error');
    }
  }

  function formatTime(ts) {
    const d = new Date(ts);
    return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
  }

  function getInitials(name) {
    return (name || 'NC').split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2);
  }

  function getStatusColor(s) {
    return { online: '#4CAF50', connecting: '#FFC107', error: '#F44336' }[s] || '#9E9E9E';
  }

  function getStatusText(s) {
    return { online: 'Онлайн', connecting: 'Подключение...', error: 'Ошибка' }[s] || 'Оффлайн';
  }

  // === Message Context Menu ===
  async function showMessageMenu(e, msg) {
    e.preventDefault();
    e.stopPropagation();
    
    // Prevent overflow
    let x = e.clientX;
    let y = e.clientY;
    const menuWidth = 200; // approx
    const menuHeight = 150; // approx
    
    if (x + menuWidth > window.innerWidth) {
        x = window.innerWidth - menuWidth - 10;
    }
    if (y + menuHeight > window.innerHeight) {
        y = window.innerHeight - menuHeight - 10;
    }

    messageContextMenu = { 
        show: true, 
        x: x, 
        y: y, 
        message: msg,
        imagePath: (e.target.tagName === 'IMG' && e.target.classList.contains('msg-img')) ? e.target.dataset.path : null,
        filePath: (e.target.closest('.file-attachment-card')) ? e.target.closest('.file-attachment-card').dataset.path : null
    };

    // Correct position if overflows
    await tick();
    const menuEl = document.querySelector('.context-menu'); // better to use bind:this but this works for now
    if (menuEl) {
        const rect = menuEl.getBoundingClientRect();
        if (rect.bottom > window.innerHeight) {
             messageContextMenu.y -= (rect.bottom - window.innerHeight + 10);
        }
    }
  }

  async function copyImageToClipboard(path) {
      if (!path) return;
      try {
          // Use backend to copy image to clipboard to avoid browser restrictions
          await CopyImageToClipboard(path);
          showToast('Картинка скопирована!', 'success');
          messageContextMenu.show = false;
      } catch (err) {
          console.error(err);
          showToast('Ошибка копирования: ' + err, 'error');
      }
  }

  async function copyPathToClipboard(path) {
      if (!path) return;
      try {
          await CopyToClipboard(path);
          showToast('Путь к файлу скопирован', 'success');
          messageContextMenu.show = false;
      } catch (err) {
          console.error(err);
          showToast('Ошибка копирования: ' + err, 'error');
      }
  }

  async function openFile(path) {
      if (!path) return;
      try {
          await OpenFile(path);
      } catch (err) {
          console.error(err);
          showToast('Не удалось открыть файл: ' + err, 'error');
      }
  }

  async function showInFolder(path) {
      if (!path) return;
      try {
          await ShowInFolder(path);
          messageContextMenu.show = false;
      } catch (err) {
          console.error(err);
          showToast('Не удалось открыть папку: ' + err, 'error');
      }
  }

  // Handle Clipboard Paste
  async function handlePaste(e) {
      console.log('Paste event triggered', e);
      // Check if we are focusing input or body.
      const items = (e.clipboardData || e.originalEvent.clipboardData).items;
      console.log('Clipboard items:', items);
      
      for (let index in items) {
          const item = items[index];
          if (item.kind === 'file' && item.type.indexOf('image/') !== -1) {
              e.preventDefault(); // Prevent default paste behavior
              console.log('Found image in clipboard');
              const blob = item.getAsFile();
              const reader = new FileReader();
              reader.onload = async (event) => {
                  try {
                      // Save to temp file via backend
                      const base64 = event.target.result; // data:image/png;base64,...
                      const path = await SaveTempImage(base64, "pasted_image.png");
                      
                      // Add to selection
                      if (selectedFiles.length < 6) {
                          selectedFiles = [...selectedFiles, path];
                          // Extract pure base64 for preview
                          const previewB64 = base64.split(',')[1];
                          filePreviews[path] = previewB64;
                      } else {
                          showToast('Максимум 6 вложений', 'error');
                      }
                  } catch (err) {
                      console.error("Paste error", err);
                      showToast('Ошибка вставки изображения', 'error');
                  }
              };
              reader.readAsDataURL(blob);
              return; // handled
          }
      }
  }


  function startEditMessage(msg) {
    editingMessageId = msg.id;
    editMessageContent = msg.content;
    messageContextMenu.show = false;
  }

  async function saveEditMessage() {
    if (!editingMessageId || !editMessageContent.trim()) return;
    try {
      await EditMessage(editingMessageId, editMessageContent);
      messages = messages.map(m => 
        m.id === editingMessageId 
          ? {...m, content: editMessageContent, edited: true} 
          : m
      );
      editingMessageId = null;
      editMessageContent = '';
    } catch (e) {
      console.error('Edit failed:', e);
      showToast('Ошибка редактирования: ' + e, 'error');
    }
  }

  function cancelEdit() {
    editingMessageId = null;
    editMessageContent = '';
  }

  async function deleteMsg(msg, forAll = false) {
    const confirmText = forAll 
      ? 'Удалить сообщение у всех?' 
      : 'Удалить сообщение у себя?';
    
    openConfirmModal(
        confirmText,
        "Это действие необратимо.",
        async () => {
            try {
              if (forAll) {
                await DeleteMessageForAll(msg.id);
              } else {
                await DeleteMessage(msg.id);
              }
              messages = messages.filter(m => m.id !== msg.id);
              messageContextMenu.show = false;
            } catch (e) {
              console.error('Delete failed:', e);
              showToast('Ошибка удаления: ' + e, 'error');
            }
        }
    );
  }

  function copyMessageText(msg) {
    CopyToClipboard(msg.content);
    messageContextMenu.show = false;
    // TODO: Toast вместо alert
  }

  // === File Transfer Logic ===
  async function acceptTransfer(msg) {
      try {
          // Optimistic update
          // messages = messages.filter(m => m.id !== msg.id); // Remove offer? No, keep logic in backend
          await AcceptFileTransfer(msg.id);
          showToast('Принято! Скачивание начнется...', 'success');
      } catch (e) {
          showToast('Ошибка принятия: ' + e, 'error');
      }
  }

  async function declineTransfer(msg) {
      try {
          await DeclineFileTransfer(msg.id);
          // Optimistic update to remove or strike-through
          // messages = messages.filter(m => m.id !== msg.id); 
          showToast('Отклонено', 'info');
      } catch (e) {
          showToast('Ошибка отклонения: ' + e, 'error');
      }
  }

  // Keyboard shortcuts
  function handleKeydown(e) {
    if (e.key === 'Escape') {
      messageContextMenu.show = false;
      contextMenu.show = false;
      cancelEdit();
      showSettings = false;
      showAddContact = false;
      showContactProfile = false;
    }
  }
  // === Folders Logic ===
  async function loadFolders() {
    try {
      // Check if GetFolders is available (might be undefined during hot reload or if backend not ready)
      if (typeof GetFolders === 'function') {
        const backendFolders = await GetFolders();
        if (backendFolders) {
            folders = backendFolders;
        } else {
            folders = [];
        }
      }
    } catch (e) {
      console.error('Failed to load folders:', e);
    }
  }
  
  async function createFolder() {
    if (!newFolderName.trim()) return;
    try {
      if (isEditingFolder && editingFolderId) {
        await UpdateFolder(editingFolderId, newFolderName, newFolderIcon);
        showToast('Папка обновлена!', 'success');
      } else {
        await CreateFolder(newFolderName, newFolderIcon);
        showToast('Папка создана!', 'success');
      }
      await loadFolders();
      showCreateFolder = false;
      newFolderName = '';
      isEditingFolder = false;
      editingFolderId = null;
    } catch (e) {
      console.error(e);
      showToast('Ошибка: ' + e, 'error');
    }
  }

  function openEditFolder(folder) {
    newFolderName = folder.name;
    newFolderIcon = folder.icon;
    editingFolderId = folder.id;
    isEditingFolder = true;
    showCreateFolder = true;
  }

  function startDeleteFolder(folder) {
    folderToDelete = folder;
    showDeleteFolderConfirm = true;
    showCreateFolder = false; // Close edit modal
  }

  async function confirmDeleteFolder() {
    if (!folderToDelete) return;
    try {
      await DeleteFolder(folderToDelete.id);
      if (activeFolderId === folderToDelete.id) activeFolderId = 'all';
      await loadFolders();
      showToast('Папка удалена', 'success');
      showDeleteFolderConfirm = false;
      folderToDelete = null;
    } catch (e) {
      showToast('Ошибка: ' + e, 'error');
    }
  }
  
  // Call loadFolders when component mounts (or when Wails is ready)
  onMount(() => {
    // Initial attempt
    setTimeout(loadFolders, 1000); 
    // Also listen for connection
    EventsOn('wails:ready', loadFolders);
  });

  // Also add helper to add chat to folder (context menu)
  async function addChatToFolder(folderId, contactId) {
      try {
          await AddChatToFolder(folderId, contactId);
          await loadFolders();
          showToast('Чат добавлен в папку', 'success');
      } catch (e) {
          showToast('Ошибка: ' + e, 'error');
      }
  }

  async function removeChatFromFolder(folderId, contactId) {
      try {
          await RemoveChatFromFolder(folderId, contactId);
          await loadFolders();
          showToast('Чат удален из папки', 'success');
      } catch (e) {
          showToast('Ошибка: ' + e, 'error');
      }
  }
  // === Avatar Logic ===
  async function handleAvatarChange(e) {
    const file = e.target.files[0];
    if (!file) return;

    if (file.size > 5 * 1024 * 1024) {
      showToast('Файл слишком большой (макс 5MB)', 'error');
      return;
    }

    try {
      const base64 = await resizeImage(file, 200, 200);
      profileAvatar = base64;
    } catch (err) {
      console.error(err);
      showToast('Ошибка обработки изображения', 'error');
    }
  }

  function resizeImage(file, maxWidth, maxHeight) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = (readerEvent) => {
        const image = new Image();
        image.onload = () => {
          const canvas = document.createElement('canvas');
          let width = image.width;
          let height = image.height;

          if (width > height) {
            if (width > maxWidth) {
              height *= maxWidth / width;
              width = maxWidth;
            }
          } else {
            if (height > maxHeight) {
              width *= maxHeight / height;
              height = maxHeight;
            }
          }

          canvas.width = width;
          canvas.height = height;
          const ctx = canvas.getContext('2d');
          ctx.drawImage(image, 0, 0, width, height);
          
          const dataUrl = canvas.toDataURL('image/jpeg', 0.7);
          resolve(dataUrl);
        };
        image.onerror = reject;
        image.src = readerEvent.target.result;
      };
      reader.onerror = reject;
      reader.readAsDataURL(file);
    });
  }
</script>

<!-- Toast Notifications -->
<div class="toast-container">
  {#each toasts as toast (toast.id)}
    <div class="toast toast-{toast.type}" class:toast-enter={true}>
      {toast.message}
    </div>
  {/each}
</div>

<!-- Context Menu for Contacts -->
{#if contextMenu.show}
<div class="context-menu" style="top: {contextMenu.y}px; left: {contextMenu.x}px" on:click|stopPropagation>
  <!-- Folder actions -->
  {#if folders.length > 0}
      <div class="context-header" style="padding: 8px 12px; font-size: 12px; opacity: 0.5;">ПАПКИ</div>
      {#each folders as folder (folder.id)}
          {#if folder.chatIds && folder.chatIds.includes(contextMenu.contact.id)}
              <div class="context-item" on:click={() => { removeChatFromFolder(folder.id, contextMenu.contact.id); contextMenu.show = false; }}>
                  ❌ Из "{folder.name}"
              </div>
          {:else}
              <div class="context-item" on:click={() => { addChatToFolder(folder.id, contextMenu.contact.id); contextMenu.show = false; }}>
                  ➕ В "{folder.name}"
              </div>
          {/if}
      {/each}
      <div class="divider"></div>
  {/if}

  <div class="context-item" on:click={deleteContactFromMenu} on:keydown={(e) => e.key === 'Enter' && deleteContactFromMenu()} role="button" tabindex="0">
    🗑️ Удалить контакт
  </div>
</div>
{/if}

<!-- Context Menu for Messages -->
{#if messageContextMenu.show && messageContextMenu.message}
<div class="context-menu" style="top: {messageContextMenu.y}px; left: {messageContextMenu.x}px" on:click|stopPropagation>
  {#if messageContextMenu.message.isOutgoing}
    <div class="context-item" on:click={() => startEditMessage(messageContextMenu.message)} on:keydown={(e) => e.key === 'Enter' && startEditMessage(messageContextMenu.message)} role="button" tabindex="0">
      ✏️ Редактировать
    </div>
    <div class="context-item" on:click={() => deleteMsg(messageContextMenu.message, true)} on:keydown={(e) => e.key === 'Enter' && deleteMsg(messageContextMenu.message, true)} role="button" tabindex="0">
      🗑️ Удалить у всех
    </div>
  {/if}
  <div class="context-item" on:click={() => deleteMsg(messageContextMenu.message, false)} on:keydown={(e) => e.key === 'Enter' && deleteMsg(messageContextMenu.message, false)} role="button" tabindex="0">
    🗑️ Удалить у себя
  </div>
  <div class="context-item" on:click={() => copyMessageText(messageContextMenu.message)} on:keydown={(e) => e.key === 'Enter' && copyMessageText(messageContextMenu.message)} role="button" tabindex="0">
    📋 Копировать текст
  </div>
  {#if messageContextMenu.imagePath}
    <div class="context-item" on:click={() => copyImageToClipboard(messageContextMenu.imagePath)} on:keydown={(e) => e.key === 'Enter' && copyImageToClipboard(messageContextMenu.imagePath)} role="button" tabindex="0">
      🖼️ Копировать изображение
    </div>
  {/if}
</div>
{/if}

<!-- Contact Profile Modal -->
{#if showContactProfile && profileContact}
<div class="modal-backdrop animate-fade-in" on:click|self={() => showContactProfile = false}>
  <div class="modal-content animate-slide-down">
     <div class="modal-header">
       <h2>👤 Профиль контакта</h2>
     </div>
     <div class="modal-body">
         <div class="profile-avatar-large" style="background: linear-gradient(135deg, hsl({profileContact.id.charCodeAt(0) * 10}, 70%, 50%), hsl({profileContact.id.charCodeAt(1) * 10}, 70%, 40%)); width: 80px; height: 80px; margin: 0 auto 16px; display: flex; align-items: center; justify-content: center; font-size: 32px; color: white; border-radius: 50%;">
            {#if profileContact.avatar}<img src={profileContact.avatar} style="width:100%;height:100%;border-radius:50%;object-fit:cover;"/>{:else}{getInitials(profileContact.nickname)}{/if}
         </div>
         <h3 style="text-align: center; margin-bottom: 20px; color: white;">{profileContact.nickname}</h3>
         
         <div class="settings-section">
            <details class="i2p-address-details">
              <summary style="color: white !important; cursor: pointer;">🔗 I2P Адрес <span class="hint" style="color: rgba(255,255,255,0.7) !important;">(нажмите чтобы показать)</span></summary>
              <div class="destination-box" style="margin-top: 10px;">
                 <code class="destination-code" style="word-break: break-all; font-size: 10px;">{profileContact.i2pAddress}</code>
              </div>
              <button class="btn-small btn-secondary" style="margin-top: 8px; width: 100%;" on:click={() => { CopyToClipboard(profileContact.i2pAddress); showToast('I2P адрес скопирован!', 'success'); }}>
                📋 Скопировать адрес
              </button>
            </details>
         </div>
     </div>
     <div class="modal-footer" style="display: flex; gap: 10px; justify-content: flex-end;">
       <button class="btn-secondary" on:click={() => showContactProfile = false}>Закрыть</button>
       <button class="btn-primary" on:click={() => requestProfileUpdate(profileContact.id)}>🔄 Обновить профиль</button>
     </div>
  </div>
</div>
{/if}

<!-- Create Folder Modal -->
{#if showCreateFolder}
<div class="modal-backdrop animate-fade-in" on:click|self={() => { showCreateFolder = false; isEditingFolder = false; editingFolderId = null; }}>
  <div class="modal-content animate-slide-down">
     <div class="modal-header">
       <h2 style="color: var(--text-primary);">📂 {isEditingFolder ? 'Редактировать папку' : 'Новая папка'}</h2>
     </div>
     <div class="modal-body">
         <div class="input-wrapper" style="margin-bottom: 20px;">
            <label style="color: var(--text-secondary); font-size: 14px; margin-bottom: 8px; display: block;">Название:</label>
            <input type="text" class="input-field" placeholder="Название папки" bind:value={newFolderName} on:keydown={(e) => e.key === 'Enter' && createFolder()} autofocus />
         </div>
         
         <div class="input-wrapper" style="margin-bottom: 20px;">
            <label style="color: var(--text-secondary); font-size: 14px; margin-bottom: 8px; display: block;">Иконка (эмодзи):</label>
            <div style="display: flex; gap: 8px;">
               <input type="text" class="input-field" placeholder="🛠️" bind:value={newFolderIcon} style="width: 60px; text-align: center; font-size: 20px;" maxlength="2" />
               <div style="flex: 1; display: flex; align-items: center; color: var(--text-secondary); font-size: 12px;">Введите любой эмодзи или выберите из списка ниже</div>
            </div>
         </div>

         <div class="folder-icons" style="display: flex; gap: 8px; overflow-x: auto; padding-bottom: 8px; margin-bottom: 10px;">
            {#each folderIcons as icon}
               <!-- svelte-ignore a11y-click-events-have-key-events -->
               <div 
                  on:click={() => newFolderIcon = icon}
                  style="font-size: 24px; cursor: pointer; padding: 8px; border-radius: 8px; background: {newFolderIcon === icon ? 'var(--accent)' : 'var(--bg-input)'}; transition: all 0.2s;"
               >
                  {icon}
               </div>
            {/each}
         </div>
     </div>
     <div class="modal-footer" style="display: flex; gap: 10px; justify-content: flex-end; margin-top: 24px;">
       {#if isEditingFolder}
         <button class="btn-danger" style="margin-right: auto;" on:click={() => { 
           const folder = folders.find(f => f.id === editingFolderId);
           startDeleteFolder(folder);
         }}>
           <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
           <span>Удалить</span>
         </button>
       {/if}
       <button class="btn-secondary" on:click={() => { showCreateFolder = false; isEditingFolder = false; editingFolderId = null; }}>Отмена</button>
       <button class="btn-primary" on:click={createFolder}>{isEditingFolder ? 'Сохранить' : 'Создать'}</button>
     </div>
  </div>
</div>
{/if}

<!-- Custom Delete Folder Confirmation Modal -->
{#if showDeleteFolderConfirm}
<div class="modal-backdrop animate-fade-in" style="z-index: 2000;">
  <div class="modal-content animate-slide-down" style="max-width: 400px; text-align: center; padding: 30px;">
     <div class="modal-header" style="justify-content: center; margin-bottom: 20px;">
       <div style="font-size: 48px; margin-bottom: 10px;">⚠️</div>
     </div>
     <h2 style="color: var(--text-primary); margin-bottom: 10px;">Удалить папку?</h2>
     <p style="color: var(--text-secondary); margin-bottom: 30px;">
       Вы уверены, что хотите удалить папку <b>"{folderToDelete?.name}"</b>? <br/>
       Чат-сессии не будут удалены.
     </p>
     <div class="modal-footer" style="display: flex; gap: 12px; justify-content: center;">
       <button class="btn-secondary" style="flex: 1;" on:click={() => { showDeleteFolderConfirm = false; folderToDelete = null; }}>Отмена</button>
       <button class="btn-danger" style="flex: 1;" on:click={confirmDeleteFolder}>Удалить</button>
     </div>
  </div>
</div>
{/if}

<!-- Login Screen -->
{#if screen === 'login'}
<div class="login-screen">
  <div class="login-container animate-fade-in">
    <div class="login-logo animate-float">
      <img src={logo} alt="TeleGhost" class="logo-img" />
    </div>
    <h1 class="login-title">TeleGhost</h1>
    <p class="login-subtitle">Анонимный мессенджер на I2P</p>
    
    <div class="login-form">
      <textarea
        class="seed-input"
        placeholder="Введите seed-фразу (12 слов)"
        bind:value={seedPhrase}
        rows="3"
      ></textarea>
      
      <button class="btn-primary animate-pulse-hover" on:click={handleLogin} disabled={isLoading}>
        {#if isLoading}
          <span class="spinner"></span>
        {:else}
          Войти
        {/if}
      </button>
      
      <div class="divider"><span>или</span></div>
      
      <button class="btn-secondary" on:click={handleCreateAccount} disabled={isLoading}>
        Создать аккаунт
      </button>
    </div>

    <p class="login-footer">🔒 Все данные хранятся локально</p>
  </div>
</div>

<!-- Mnemonic Modal -->
{#if showMnemonicModal}
<div class="modal-backdrop animate-fade-in">
  <div class="modal-content animate-slide-down">
    <div class="modal-header">
      <h2>🔐 Ваш секретный ключ</h2>
    </div>
    <div class="modal-body">
      <p class="warning-text">⚠️ Сохраните эти 12 слов. Без них восстановить доступ невозможно!</p>
      
      <div class="mnemonic-grid">
        {#each newMnemonic.split(' ') as word, i}
          <div class="mnemonic-word">
            <span class="word-index">{i+1}</span>
            <span class="word-text">{word}</span>
          </div>
        {/each}
      </div>

      <div class="mnemonic-actions">
         <button class="btn-text" on:click={() => { CopyToClipboard(newMnemonic); showToast('Скопировано!', 'success'); }}>
           📋 Скопировать всё
         </button>
      </div>
    </div>
    <div class="modal-footer">
      <button class="btn-primary full-width" on:click={confirmMnemonicSaved}>
        Я сохранил(а) seed-фразу
      </button>
    </div>
  </div>
</div>
{/if}

{:else}
<!-- Main Screen -->
<div class="main-screen">
  <!-- Sidebar -->
  <!-- Folders Rail -->
  <div class="folders-rail">
    <div class="rail-button" on:click={() => showSettings = true}>
      <div class="hamburger-icon">
        <span></span><span></span><span></span>
      </div>
    </div>

    <div class="folders-list">
      {#each uiFolders as folder}
        <div 
          class="folder-item" 
          class:active={activeFolderId === folder.id && folder.id !== 'add'} 
          on:click={() => {
            if (folder.id === 'add') {
               showCreateFolder = true;
               isEditingFolder = false;
               editingFolderId = null;
               newFolderName = '';
               newFolderIcon = '📁';
            } else {
               activeFolderId = folder.id;
            }
          }}
          on:contextmenu|preventDefault={(e) => {
            if (folder.id !== 'all' && folder.id !== 'add') {
               openEditFolder(folder);
            }
          }}
          title={folder.id === 'add' ? 'Создать папку' : folder.name}
          style={folder.id === 'add' ? 'margin-top: 10px; opacity: 0.7;' : ''}
        >
          <div class="folder-icon">{folder.icon}</div>
          {#if folder.id === 'all'}
             <div class="folder-name">Все чаты</div>
          {/if}
        </div>
      {/each}
    </div>

    <div class="rail-button bottom" on:click={() => showSettings = true} title="Настройки">
      <div class="settings-icon">⚙️</div>
    </div>
  </div>

  <!-- Sidebar (Resizable) -->
  <div class="sidebar" style="width: {sidebarWidth}px; min-width: 240px; flex: none; display: flex; flex-direction: column;">
    <div class="sidebar-header" style="padding: 10px; background: var(--bg-secondary);">
      <div class="search-input-wrapper" style="background: var(--bg-input); border-radius: 18px; padding: 8px 12px; display: flex; align-items: center; gap: 8px;">
        <span class="search-icon" style="opacity: 0.5;">🔍</span>
        <input type="text" placeholder="Поиск" bind:value={searchQuery} style="background: transparent; border: none; color: white; width: 100%; font-size: 14px; outline: none; font-family: inherit;" />
      </div>
    </div>
    
    <div class="sidebar-actions" style="display: flex; gap: 10px; padding: 0 10px 10px;">
       <button class="btn-primary" on:click={() => showAddContact = !showAddContact} title="Добавить контакт / Новый чат" style="flex: 1; display: flex; align-items: center; justify-content: center; gap: 8px; padding: 8px;">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/></svg>
          <span style="font-weight: 500;">Новый чат</span>
       </button>
    </div>
    
    <!-- Network Status -->
    <div class="network-status" style="background: {getStatusColor(networkStatus)}15">
      <div class="status-dot animate-pulse" style="background: {getStatusColor(networkStatus)}"></div>
      <span>{getStatusText(networkStatus)}</span>
    </div>

    <!-- Add Contact Panel -->
    {#if showAddContact}
    <div class="add-contact-panel animate-slide-down">
      <input type="text" placeholder="Имя контакта" bind:value={addContactName} class="input-field" />
      <textarea placeholder="I2P адрес" bind:value={addContactAddress} rows="2" class="input-field"></textarea>
      <div class="add-contact-actions">
        <button class="btn-small btn-secondary" on:click={() => showAddContact = false}>Отмена</button>
        <button class="btn-small btn-primary" on:click={handleAddContactManual}>Добавить</button>
      </div>
      <button class="btn-clipboard" on:click={handleAddContactFromClipboard}>
        📋 Из буфера обмена
      </button>
    </div>
    {/if}
    
    <!-- Contacts List -->
    <div class="contacts-list">
      {#each filteredContacts as contact}
        <div 
          class="contact-item animate-card" 
          class:selected={selectedContact && selectedContact.id === contact.id} 
          on:click={() => selectContact(contact)}
          on:contextmenu|preventDefault={(e) => handleContextMenu(e, contact)}
          on:keypress={(e) => e.key === 'Enter' && selectContact(contact)}
          tabindex="0"
          role="button"
        >
          <div class="contact-avatar" style="background: linear-gradient(135deg, hsl({contact.id.charCodeAt(0) * 10}, 70%, 50%), hsl({contact.id.charCodeAt(1) * 10}, 70%, 40%))">
            {#if contact.avatar}<img src={contact.avatar} style="width:100%;height:100%;border-radius:50%;object-fit:cover;"/>{:else}{getInitials(contact.nickname)}{/if}
          </div>
          <div class="contact-info">
            <div class="contact-name" style="color: white !important;">{contact.nickname}</div>
            <div class="contact-last">{contact.lastMessage || 'Нет сообщений'}</div>
          </div>
        </div>
      {/each}
      
      {#if contacts.length === 0}
        <div class="no-contacts animate-fade-in">
          <div class="no-contacts-icon">👻</div>
          <p>Нет контактов</p>
          <p class="hint">Нажмите + чтобы добавить</p>
        </div>
      {/if}
    </div>
    
    <!-- My Destination -->
    <div class="my-destination">
      <button class="btn-copy" on:click={copyDestination}>
        <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
          <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
        </svg>
        <span>Копировать мой I2P адрес</span>
      </button>
    </div>
  </div>
  
  <div class="resizer" on:mousedown={startResize} class:resizing={isResizing}></div>

  <!-- Main Content -->
  <div class="content-area">
    {#if showSettings}
      <!-- Settings Panel -->
      <div class="settings-panel animate-fade-in" style="flex-direction: row; padding: 0;">
        
        <!-- Left Menu -->
        <div class="settings-sidebar" style="width: 250px; min-width: 250px; background: var(--bg-secondary); border-right: 1px solid var(--border); display: flex; flex-direction: column;">
           <div class="settings-sidebar-header" style="padding: 24px; display: flex; align-items: center; gap: 10px;">
              <button class="btn-icon" on:click={() => showSettings = false} title="Назад" style="background: transparent;">
                <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z"/></svg>
              </button>
              <h2 style="font-size: 20px; font-weight: 600; margin: 0; color: white !important;">Настройки</h2>
           </div>

           <div class="settings-menu" style="display: flex; flex-direction: column; padding: 0 10px; gap: 4px;">
              {#each settingsCategories as cat}
                <div 
                   class="settings-menu-item" 
                   class:active={activeSettingsTab === cat.id}
                   on:click={() => activeSettingsTab = cat.id}
                   style="display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-radius: 8px; cursor: pointer; color: {activeSettingsTab === cat.id ? 'white' : 'var(--text-secondary)'}; background: {activeSettingsTab === cat.id ? 'var(--accent)' : 'transparent'}; transition: all 0.2s;"
                >
                   <span class="icon" style="font-size: 18px;">{cat.icon}</span>
                   <span class="name" style="font-weight: 500;">{cat.name}</span>
                </div>
              {/each}
           </div>
        </div>

        <!-- Right Content -->
        <div class="settings-content-area" style="flex: 1; padding: 40px; overflow-y: auto; color: var(--text-primary);">
           {#if activeSettingsTab === 'profile'}
              <!-- Profile Content -->
              <h3 style="margin-bottom: 24px; font-size: 24px; color: var(--text-primary);">Аккаунт</h3>
              <div class="settings-section">
                <!-- Avatar -->
                <div class="profile-avatar-large" style="width: 120px; height: 120px; position: relative;">
                    {#if profileAvatar}
                      <img src={profileAvatar} alt="Avatar" style="width: 100%; height: 100%; object-fit: cover; border-radius: 50%; box-shadow: 0 5px 15px rgba(0,0,0,0.3);" />
                    {:else}
                      <div style="width: 100%; height: 100%; background: linear-gradient(135deg, #6c5ce7, #a29bfe); border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 48px; color: white; box-shadow: 0 5px 15px rgba(0,0,0,0.3);">
                        {getInitials(profileNickname)}
                      </div>
                    {/if}
                    <button class="avatar-edit-btn animate-pop" style="position: absolute; bottom: 0; right: 0; background: var(--accent); border: 4px solid var(--bg-primary); border-radius: 50%; width: 40px; height: 40px; display: flex; align-items: center; justify-content: center; cursor: pointer; color: white; box-shadow: 0 2px 5px rgba(0,0,0,0.2);" on:click={() => avatarFileInput.click()}>📷</button>
                    <input type="file" bind:this={avatarFileInput} on:change={handleAvatarChange} accept="image/*" style="display: none;" />
                </div>
                
                <div class="profile-fields" style="margin-top: 24px; max-width: 400px;">
                  <label class="form-label" style="color: var(--text-primary);">Никнейм
                    <input type="text" bind:value={profileNickname} class="input-field" placeholder="Ваш никнейм" />
                  </label>
                  <label class="form-label" style="margin-top: 16px; color: var(--text-primary);">О себе
                    <textarea bind:value={profileBio} class="input-field" rows="3" placeholder="Расскажите о себе..."></textarea>
                  </label>
                  <button class="btn-primary" style="margin-top: 24px;" on:click={saveProfile}>💾 Сохранить изменения</button>
                </div>
              </div>

           {:else if activeSettingsTab === 'chats'}
              <h3 style="margin-bottom: 24px; font-size: 24px; color: var(--text-primary);">Настройки чатов</h3>
              <div class="settings-section">
                 <p class="hint" style="color: var(--text-secondary);">Здесь будут настройки темы, шрифта и фона чата.</p>
                 <div class="mock-setting" style="margin-top: 20px; opacity: 0.5;">
                    <label style="color: var(--text-primary);">Размер шрифта</label>
                    <input type="range" min="12" max="20" value="14" disabled />
                 </div>
              </div>

           {:else if activeSettingsTab === 'privacy'}
              <h3 style="margin-bottom: 24px; font-size: 24px; color: var(--text-primary);">Конфиденциальность</h3>
              <div class="settings-section">
                 <div class="info-box" style="background: rgba(255, 100, 100, 0.1); padding: 16px; border-radius: 8px; border: 1px solid rgba(255, 100, 100, 0.3);">
                    <h4 style="color: #ff6b6b; margin: 0 0 8px;">🔐 Секретный ключ (Seed phrase)</h4>
                    <p style="font-size: 13px; color: var(--text-secondary);">Ваш ключ хранится только на этом устройстве. Если вы потеряете его, доступ к аккаунту будет утерян навсегда.</p>
                    <button class="btn-secondary" style="margin-top: 10px;" on:click={() => showToast('Функция показа ключа в разработке', 'warning')}>Показать ключ</button>
                 </div>
              </div>

           {:else if activeSettingsTab === 'network'}
              <h3 style="margin-bottom: 24px; font-size: 24px; color: var(--text-primary);">Сеть и I2P</h3>
              <div class="settings-section">
                 <label class="form-label" style="color: var(--text-primary);">Ваш I2P адрес (Destination)</label>
                 <div class="destination-box">
                   <code class="destination-code">{myDestination ? myDestination.slice(0, 50) + '...' : 'Загрузка...'}</code>
                   <button class="btn-icon-small" on:click={copyDestination}>📋</button>
                 </div>
                 <div class="info-item" style="margin-top: 20px;">
                  <span class="info-label" style="color: var(--text-primary);">Статус сети:</span>
                  <span class="info-value" style="color: {getStatusColor(networkStatus)}">{getStatusText(networkStatus)}</span>
                 </div>

                 <h4 style="margin-top: 24px; color: var(--text-primary); border-top: 1px solid var(--border); padding-top: 20px;">Настройки роутера</h4>
                 <div class="settings-section">
                    <!-- Tunnel Length -->
                    <div class="setting-item" style="margin-bottom: 20px;">
                        <label class="form-label" style="color: var(--text-primary); display: block; margin-bottom: 8px;">Режим анонимности (длина туннелей)</label>
                        <select bind:value={routerSettings.tunnelLength} class="input-field settings-select">
                            <option value={1}>Fast (1 хоп) - Максимальная скорость, низкая анонимность</option>
                            <option value={2}>Normal (2 хопа) - Баланс (Рекомендуется)</option>
                            <option value={4}>Invisible (4 хопа) - Высокая анонимность, медленно</option>
                        </select>
                    </div>

                    <!-- Logging -->
                    <div class="setting-item" style="margin-bottom: 20px; display: flex; align-items: center; justify-content: space-between;">
                        <div>
                            <span style="color: var(--text-primary); font-weight: 500;">Логирование в файл</span>
                            <p style="margin: 4px 0 0; font-size: 13px; color: var(--text-secondary);">Записывать логи роутера в i2pd.log</p>
                        </div>
                        <input type="checkbox" bind:checked={routerSettings.logToFile} style="transform: scale(1.5); cursor: pointer;" />
                    </div>

                    <button class="btn-primary" on:click={saveRouterSettings} style="width: 100%;">💾 Сохранить и применить</button>
                    <p style="margin-top: 10px; font-size: 12px; color: var(--text-secondary); text-align: center;">Для вступления в силу изменений потребуется перезапуск приложения.</p>
                 </div>
              </div>

           {:else if activeSettingsTab === 'about'}
              <h3 style="margin-bottom: 24px; font-size: 24px; color: var(--text-primary);">О программе</h3>
              <div class="info-grid">
                <div class="info-item"><span class="info-label">Версия</span><span class="info-value">1.1.0-beta</span></div>
                <div class="info-item"><span class="info-label">Разработчик</span><span class="info-value">TeleGhost Team</span></div>
                <div class="info-item"><span class="info-label">Лицензия</span><span class="info-value">MIT / Open Source</span></div>
              </div>
           {/if}
        </div>
      </div>

    {:else if selectedContact}
      <!-- Chat -->
      <div class="chat-area animate-fade-in">
        <div class="chat-header">
          <div class="chat-contact-info" on:click={openContactProfile} style="cursor: pointer;">
            <div class="chat-avatar" style="background: linear-gradient(135deg, hsl({selectedContact.id.charCodeAt(0) * 10}, 70%, 50%), hsl({selectedContact.id.charCodeAt(1) * 10}, 70%, 40%))">
              {#if selectedContact.avatar}<img src={selectedContact.avatar} style="width:100%;height:100%;border-radius:50%;object-fit:cover;"/>{:else}{getInitials(selectedContact.nickname)}{/if}
            </div>
            <div>
              <div class="chat-name" style="color: white !important;">{selectedContact.nickname}</div>
              <div class="chat-status" style="display: flex; align-items: center; gap: 6px;">
                <span style="width: 8px; height: 8px; border-radius: 50%; background: {messages.some(m => !m.isOutgoing && m.senderId === selectedContact.publicKey && (Date.now() - new Date(m.timestamp).getTime() < 300000)) ? '#4CAF50' : '#9E9E9E'};"></span>
                <span style="font-size: 12px; color: var(--text-secondary);">
                  {messages.some(m => !m.isOutgoing && m.senderId === selectedContact.publicKey && (Date.now() - new Date(m.timestamp).getTime() < 300000)) ? 'В сети' : 'Оффлайн'}
                </span>
              </div>
            </div>
          </div>
        </div>
        
        <div class="messages-container">
          {#each messages as msg (msg.id)}
            <div class="message animate-message" class:outgoing={msg.isOutgoing}>
              <div class="message-bubble" class:outgoing={msg.isOutgoing} on:contextmenu={(e) => showMessageMenu(e, msg)}>
                <!-- Image Rendering -->
                {#if msg.attachments && msg.attachments.length > 0}
                  <div class="message-images" style="grid-template-columns: {msg.attachments.length === 1 ? '1fr' : 'repeat(2, 1fr)'}">
                      {#each msg.attachments as att}
                         {@const ext = att.filename ? att.filename.split('.').pop().toLowerCase() : (att.local_path ? att.local_path.split('.').pop().toLowerCase() : '')}
                         {@const isImg = ['jpg','jpeg','png','webp','gif','bmp'].includes(ext) || (att.mimeType && att.mimeType.startsWith('image/'))}

                         {#if isImg}
                             <!-- svelte-ignore a11y-click-events-have-key-events -->
                             <img 
                                 use:startLoadingImage={att.local_path} 
                                 alt="attachment" 
                                 class="msg-img" 
                                 data-path={att.local_path}
                                 style="height: {msg.attachments.length === 1 ? 'auto' : '120px'}" 
                                 on:click={(e) => previewImage = e.currentTarget.src}
                             />
                         {:else}
                             <div class="file-attachment-card" on:click={() => openFile(att.local_path)} on:contextmenu|preventDefault={(e) => showMessageMenu(e, msg)} title="Нажмите чтобы открыть файл">
                                 <div class="file-icon">📄</div>
                                 <div class="file-details">
                                     <div class="file-name">{att.filename || 'File'}</div>
                                     <div class="file-size">{att.size ? (att.size / 1024).toFixed(1) + ' KB' : ''}</div>
                                 </div>
                             </div>
                         {/if}
                      {/each}
                  </div>
                {/if}
                {#if editingMessageId === msg.id}
                  <div class="message-edit-container" on:click|stopPropagation>
                    <textarea 
                      class="message-edit-input"
                      bind:value={editMessageContent}
                      on:keydown={(e) => {
                        if (e.key === 'Enter' && !e.shiftKey) {
                          e.preventDefault();
                          saveEditMessage();
                        }
                        if (e.key === 'Escape') {
                          cancelEdit();
                        }
                      }}
                    ></textarea>
                    <div class="message-edit-actions">
                      <button class="btn-sm btn-primary" on:click={saveEditMessage}>✓</button>
                      <button class="btn-sm btn-secondary" on:click={cancelEdit}>✕</button>
                    </div>
                  </div>
                {:else}
                  {#if msg.contentType === 'file_offer'}
                      <div class="file-offer-card">
                          <div class="file-icon-large">📁</div>
                          <div class="file-info">
                              <div class="file-title">Предложено файлов: {msg.fileCount || (msg.filenames ? msg.filenames.length : '?')}</div>
                              <div class="file-size">Общий размер: {msg.totalSize ? (msg.totalSize / 1024 / 1024).toFixed(2) + ' MB' : 'Неизвестно'}</div>
                              {#if msg.filenames && msg.filenames.length > 0}
                                  <div class="file-list-preview" style="font-size: 11px; opacity: 0.7; margin-top: 4px;">
                                      {msg.filenames.slice(0, 3).join(', ')}{msg.filenames.length > 3 ? '...' : ''}
                                  </div>
                              {/if}
                          </div>
                      </div>
                      
                      <div class="file-actions" style="margin-top: 10px; display: flex; gap: 8px;">
                          {#if msg.isOutgoing}
                              <div class="status-badge">⏳ Ожидание подтверждения...</div>
                          {:else}
                              <button class="btn-small btn-success" on:click={() => acceptTransfer(msg)}>✅ Принять</button>
                              <button class="btn-small btn-danger" on:click={() => declineTransfer(msg)}>❌ Отклонить</button>
                          {/if}
                      </div>
                  {:else}
                      <div class="message-content">{@html parseMarkdown(msg.content)}</div>
                  {/if}
                {/if}
                <div class="message-meta">
                  {#if msg.edited}
                    <span class="message-edited">изм.</span>
                  {/if}
                  <span class="message-time">{formatTime(msg.timestamp)}</span>
                  {#if msg.isOutgoing}
                    <span class="message-status">{msg.status === 'sending' ? '🕐' : '✓'}</span>
                  {/if}
                </div>
              </div>
            </div>
          {/each}
          
          {#if messages.length === 0}
            <div class="no-messages animate-fade-in">
              <div class="no-messages-icon">💬</div>
              <p>Начните переписку!</p>
            </div>
          {/if}
        </div>
        
        <div class="input-area-wrapper">
          {#if selectedFiles.length > 0}
            <div class="attachment-preview animate-slide-down">
               {#each selectedFiles as file, i}
                   <div class="preview-item">
                       {#if filePreviews[file]}
                           <img src={`data:image/png;base64,${filePreviews[file]}`} alt="preview" />
                       {:else}
                           <div class="file-icon-preview">📄</div>
                           <div class="file-name-preview" style="font-size: 10px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 60px;">{file.split(/[\\/]/).pop()}</div>
                       {/if}
                       <button class="btn-remove-att" on:click={() => removeFile(i)}>X</button>
                   </div>
               {/each}
               <div class="compress-opt">
                   <label style="cursor: pointer; display: flex; align-items: center; gap: 4px;" title="Сжать изображения (для файлов игнорируется)">
                       <input type="checkbox" bind:checked={isCompressed}>
                       Сжать
                   </label>
               </div>
            </div>
          {/if}
          
          <div class="input-area">
            <button class="btn-icon" on:click={handleSelectFiles} title="Прикрепить файл">
                <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M16.5 6v11.5c0 2.21-1.79 4-4 4s-4-1.79-4-4V5a2.5 2.5 0 0 1 5 0v10.5a.5.5 0 0 1-1 0V5a1.5 1.5 0 0 0-3 0v12.5c0 1.38 1.12 2.5 2.5 2.5 1.38 0 2.5-1.12 2.5-2.5V6a.5.5 0 0 1 1 0z"/></svg>
            </button>
          
          <div style="flex: 1; position: relative;">
              <textarea
                class="message-input"
                placeholder="Сообщение..."
                bind:value={newMessage}
                on:keypress={handleKeyPress}
                on:paste={handlePaste}
                rows="1"
                maxlength="4096"
                style="width: 100%;"
              ></textarea>
              {#if newMessage.length > 3000}
                 <div class="char-counter" style="position: absolute; bottom: 5px; right: 10px; font-size: 10px; color: {newMessage.length >= 4096 ? '#ff4757' : 'var(--text-secondary)'}; background: rgba(0,0,0,0.5); padding: 2px 4px; border-radius: 4px;">
                     {newMessage.length}/4096
                 </div>
              {/if}
          </div>
          <button class="btn-send" on:click={sendMessage} disabled={!newMessage.trim() && selectedFiles.length === 0}>
            <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
              <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/>
            </svg>
          </button>
        </div>
      </div>
    </div>

    {:else}
      <!-- No Chat Selected -->
      <div class="no-chat animate-fade-in">
        <div class="no-chat-logo animate-float">
          <img src={logo} alt="TeleGhost" />
        </div>
        <h2>TeleGhost</h2>
        <p>Выберите чат или создайте новый контакт</p>
      </div>
    {/if}
  </div>
</div>
{/if}

<style>
  /* === CSS Variables === */
  :root {
    --bg-primary: #0f1923;
    --bg-secondary: #17212b;
    --bg-tertiary: #1e2c3a;
    --bg-input: #242f3d;
    --bg-message-out: #2b5278;
    --bg-message-in: #182533;
    --text-primary: #ffffff;
    --text-secondary: #8b9ba5;
    --accent: #6c5ce7;
    --accent-light: #a29bfe;
    --border: #0e1621;
    --radius: 16px;
    --radius-sm: 12px;
    --radius-xs: 8px;
  }

  * { box-sizing: border-box; margin: 0; padding: 0; }

  /* === Animations === */
  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(10px); }
    to { opacity: 1; transform: translateY(0); }
  }

  @keyframes float {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-10px); }
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }

  @keyframes slideDown {
    from { opacity: 0; transform: translateY(-10px); }
    to { opacity: 1; transform: translateY(0); }
  }

  @keyframes messageIn {
    from { opacity: 0; transform: scale(0.9) translateY(10px); }
    to { opacity: 1; transform: scale(1) translateY(0); }
  }

  .animate-fade-in { animation: fadeIn 0.4s ease-out; }
  .animate-float { animation: float 3s ease-in-out infinite; }
  .animate-pulse { animation: pulse 2s ease-in-out infinite; }
  .animate-slide-down { animation: slideDown 0.3s ease-out; }
  .animate-message { animation: messageIn 0.3s ease-out; }
  .animate-pulse-hover:hover { animation: pulse 1s ease-in-out infinite; }

  /* === Context Menu === */
  .context-menu {
    position: fixed;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    box-shadow: 0 5px 15px rgba(0,0,0,0.5);
    z-index: 10000; /* High z-index */
    overflow: hidden;
    min-width: 150px;
  }

  .context-item {
    padding: 12px 16px;
    cursor: pointer;
    font-size: 14px;
    color: var(--text-primary);
    transition: background 0.2s;
  }

  .context-item:hover {
    background: var(--bg-tertiary);
  }

  /* === Login Screen === */
  .login-screen {
    width: 100vw;
    height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(135deg, var(--bg-primary) 0%, var(--bg-secondary) 50%, #1a1a2e 100%);
  }

  .login-container {
    width: 100%;
    max-width: 420px;
    padding: 48px;
    text-align: center;
    background: var(--bg-secondary);
    border-radius: var(--radius);
    box-shadow: 0 20px 60px rgba(0,0,0,0.5);
    border: 1px solid var(--border);
  }

  .login-logo { margin-bottom: 24px; }
  .logo-img { width: 100px; height: 100px; border-radius: 50%; }

  .login-title {
    font-size: 36px;
    font-weight: 700;
    color: var(--text-primary);
    margin-bottom: 8px;
    background: linear-gradient(135deg, var(--accent-light), var(--accent));
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
  }

  .login-subtitle { color: var(--text-secondary); margin-bottom: 32px; }
  .login-form { display: flex; flex-direction: column; gap: 16px; }

  .seed-input, .input-field {
    width: 100%;
    padding: 16px;
    border: 2px solid transparent;
    border-radius: var(--radius-sm);
    background: var(--bg-input);
    color: var(--text-primary);
    font-size: 14px;
    resize: none;
    outline: none;
    transition: border-color 0.3s, box-shadow 0.3s;
  }

  /* === Modal === */
  .modal-backdrop {
    position: fixed;
    top: 0; left: 0;
    width: 100vw; height: 100vh;
    background: rgba(0, 0, 0, 0.8);
    backdrop-filter: blur(5px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal-content {
    background: var(--bg-secondary);
    border-radius: var(--radius);
    padding: 32px;
    width: 90%;
    max-width: 500px;
    box-shadow: 0 20px 60px rgba(0,0,0,0.5);
    border: 1px solid var(--border);
  }

  .modal-header h2 {
    font-size: 24px;
    margin-bottom: 16px;
    text-align: center;
    color: var(--text-primary);
  }

  .warning-text {
    background: rgba(244, 67, 54, 0.1);
    color: #ff6b6b;
    padding: 12px;
    border-radius: var(--radius-sm);
    font-size: 14px;
    margin-bottom: 24px;
    text-align: center;
  }

  .mnemonic-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 24px;
  }

  .mnemonic-word {
    background: var(--bg-input);
    padding: 8px 12px;
    border-radius: var(--radius-xs);
    display: flex;
    align-items: center;
    gap: 8px;
    font-family: monospace;
    font-size: 14px;
  }

  .word-index { color: var(--text-secondary); opacity: 0.5; font-size: 12px; }
  .word-text { color: var(--text-primary); font-weight: bold; }

  .mnemonic-actions {
    display: flex;
    justify-content: center;
    margin-bottom: 24px;
  }

  .btn-text {
    background: none;
    border: none;
    color: var(--accent-light);
    cursor: pointer;
    font-size: 14px;
  }

  .btn-text:hover { text-decoration: underline; }
  .full-width { width: 100%; }

  .seed-input:focus, .input-field:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(108, 92, 231, 0.2);
  }

  .seed-input::placeholder, .input-field::placeholder { color: var(--text-secondary); }

  .btn-primary, .btn-secondary {
    width: 100%;
    padding: 16px;
    border: none;
    border-radius: var(--radius-sm);
    font-size: 16px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.3s;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }

  .btn-primary {
    background: linear-gradient(135deg, var(--accent), #5a4fcf);
    color: white;
  }

  .btn-primary:hover { transform: translateY(-2px); box-shadow: 0 8px 25px rgba(108, 92, 231, 0.4); }
  .btn-primary:disabled { opacity: 0.6; cursor: not-allowed; transform: none; }

  .btn-secondary {
    background: transparent;
    border: 2px solid var(--accent);
    color: var(--accent);
  }

  .btn-secondary:hover { background: rgba(108, 92, 231, 0.1); }

  .btn-danger {
    padding: 16px;
    border: none;
    border-radius: var(--radius-sm);
    font-size: 16px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.3s;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    background: linear-gradient(135deg, #ff4757, #ff6b81);
    color: white;
  }
  .btn-danger:hover { transform: translateY(-2px); box-shadow: 0 8px 25px rgba(255, 71, 87, 0.4); }

  .btn-small { padding: 10px 16px; font-size: 14px; }

  .divider {
    display: flex;
    align-items: center;
    gap: 12px;
    color: var(--text-secondary);
    font-size: 13px;
  }

  .divider::before, .divider::after {
    content: '';
    flex: 1;
    height: 1px;
    background: var(--border);
  }

  .login-footer { margin-top: 24px; font-size: 12px; color: var(--text-secondary); }

  .spinner {
    width: 20px;
    height: 20px;
    border: 2px solid transparent;
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin { to { transform: rotate(360deg); } }

  /* === Main Screen === */
  .main-screen {
    width: 100vw;
    height: 100vh;
    display: flex;
    background: var(--bg-primary);
  }

  /* === Sidebar === */
  .sidebar {
    width: 320px;
    min-width: 280px;
    height: 100%;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
  }

  .sidebar-header {
    padding: 16px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border);
  }

  .sidebar-logo {
    display: flex;
    align-items: center;
    gap: 10px;
    cursor: pointer;
    font-weight: 700;
    font-size: 18px;
    color: var(--text-primary);
    transition: color 0.3s;
  }

  .sidebar-logo:hover { color: var(--accent); }
  .sidebar-logo-img { width: 36px; height: 36px; border-radius: 50%; }

  .sidebar-actions { display: flex; gap: 4px; }

  .btn-icon {
    width: 40px;
    height: 40px;
    border: none;
    border-radius: 50%;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.3s;
  }

  .btn-icon:hover { background: var(--bg-input); color: var(--accent); transform: scale(1.1); }

  .btn-icon-small {
    width: 32px;
    height: 32px;
    border: none;
    border-radius: var(--radius-xs);
    background: var(--bg-input);
    cursor: pointer;
    transition: all 0.3s;
  }

  .btn-icon-small:hover { background: var(--accent); }

  .network-status {
    padding: 10px 16px;
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 13px;
    color: var(--text-secondary);
    border-radius: var(--radius-xs);
    margin: 8px;
  }

  .status-dot { width: 10px; height: 10px; border-radius: 50%; }

  .add-contact-panel {
    padding: 16px;
    background: var(--bg-tertiary);
    margin: 0 8px 8px;
    border-radius: var(--radius-sm);
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .add-contact-actions { display: flex; gap: 8px; }
  .add-contact-actions button { flex: 1; }

  .btn-clipboard {
    padding: 10px;
    border: 1px dashed var(--accent);
    border-radius: var(--radius-xs);
    background: transparent;
    color: var(--accent);
    cursor: pointer;
    transition: all 0.3s;
  }

  .btn-clipboard:hover { background: rgba(108, 92, 231, 0.1); }

  .contacts-list { flex: 1; overflow-y: auto; }

  .contact-item {
    padding: 12px 16px;
    display: flex;
    align-items: center;
    gap: 12px;
    cursor: pointer;
    transition: all 0.3s;
    border-left: 3px solid transparent;
  }

  .contact-item:hover { background: var(--bg-tertiary); }
  .contact-item.selected { background: rgba(108, 92, 231, 0.2); border-left-color: var(--accent); }

  .contact-avatar {
    width: 50px;
    height: 50px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
    font-weight: 600;
    font-size: 16px;
    flex-shrink: 0;
  }

  .contact-info { flex: 1; min-width: 0; }
  .contact-name { color: var(--text-primary); font-weight: 500; margin-bottom: 4px; }
  .contact-last {
    color: var(--text-secondary);
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .no-contacts {
    padding: 60px 20px;
    text-align: center;
    color: var(--text-secondary);
  }

  .no-contacts-icon { font-size: 48px; margin-bottom: 16px; }
  .hint { font-size: 12px; margin-top: 8px; opacity: 0.7; }

  .my-destination { padding: 12px; border-top: 1px solid var(--border); }

  .btn-copy {
    width: 100%;
    padding: 14px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    border: 1px dashed var(--text-secondary);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font-size: 13px;
    cursor: pointer;
    transition: all 0.3s;
  }

  .btn-copy:hover { border-color: var(--accent); color: var(--accent); }

  /* === Content Area === */
  .content-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    background: var(--bg-primary);
    overflow: hidden;
    min-height: 0;
  }

  /* === Settings === */
  .settings-panel { flex: 1; overflow-y: auto; }

  .settings-header {
    padding: 24px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-secondary);
  }

  .settings-header h2 { color: var(--text-primary); font-size: 24px; }

  .settings-content { padding: 24px; max-width: 600px; }

  .profile-section {
    display: flex;
    gap: 24px;
    margin-bottom: 32px;
    flex-wrap: wrap;
  }

  .profile-avatar-large {
    width: 120px;
    height: 120px;
    position: relative;
    flex-shrink: 0;
  }

  .avatar-img {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
    border: 3px solid var(--accent);
  }

  .avatar-edit-btn {
    position: absolute;
    bottom: 0;
    right: 0;
    width: 36px;
    height: 36px;
    border: none;
    border-radius: 50%;
    background: var(--accent);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 16px;
    transition: transform 0.3s;
  }

  .avatar-edit-btn:hover { transform: scale(1.1); }

  .profile-form { flex: 1; min-width: 250px; display: flex; flex-direction: column; gap: 16px; }

  .form-label {
    display: flex;
    flex-direction: column;
    gap: 8px;
    color: var(--text-secondary);
    font-size: 13px;
  }

  .settings-section {
    margin-bottom: 24px;
    padding: 20px;
    background: var(--bg-secondary);
    border-radius: var(--radius-sm);
  }

  .settings-section h3 { color: var(--text-primary); margin-bottom: 16px; font-size: 16px; }

  .destination-box {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px;
    background: var(--bg-input);
    border-radius: var(--radius-xs);
    margin-bottom: 8px;
  }

  .destination-code {
    flex: 1;
    font-size: 11px;
    color: var(--text-secondary);
    word-break: break-all;
  }

  .info-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px; }

  .info-item {
    padding: 12px;
    background: var(--bg-input);
    border-radius: var(--radius-xs);
  }

  .info-label { display: block; font-size: 12px; color: var(--text-secondary); margin-bottom: 4px; }
  .info-value { font-size: 14px; font-weight: 600; color: var(--text-primary); }

  /* === Chat === */
  .chat-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-height: 0;
  }

  .chat-header {
    padding: 14px 20px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-secondary);
  }

  .chat-contact-info { display: flex; align-items: center; gap: 14px; }

  .chat-avatar {
    width: 46px;
    height: 46px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
    font-weight: 600;
  }

  .chat-name { font-weight: 600; font-size: 16px; margin-bottom: 4px; }
  .chat-status {
      font-size: 13px; color: var(--text-secondary);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 400px;
  }

  .messages-container {
    flex: 1;
    overflow-y: auto;
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-height: 0;
  }

  .message { display: flex; justify-content: flex-start; }
  .message.outgoing { justify-content: flex-end; }

  .message-bubble {
    max-width: 65%;
    padding: 12px 16px;
    border-radius: var(--radius);
    background: var(--bg-message-in);
    border-bottom-left-radius: 4px;
  }

  .message-bubble.outgoing {
    background: linear-gradient(135deg, var(--bg-message-out), #234567);
    border-bottom-left-radius: var(--radius);
    border-bottom-right-radius: 4px;
  }

  .message-content { 
    color: var(--text-primary); 
    line-height: 1.5; 
    word-wrap: break-word; 
    white-space: pre-wrap;
  }
  
  /* Images inside bubble */
  .message-images {
      margin-bottom: 4px;
  }

  /* Markdown Styles */
  :global(.message-content pre.md-code-block) {
    background: rgba(0, 0, 0, 0.3);
    padding: 8px;
    border-radius: 6px;
    overflow-x: auto;
    font-family: monospace;
    font-size: 13px;
    margin: 6px 0;
  }
  
  :global(.message-content code.md-code) {
    background: rgba(0, 0, 0, 0.3);
    padding: 2px 4px;
    border-radius: 4px;
    font-family: monospace;
    font-size: 13px;
  }

  :global(.message-content a) {
    color: #a29bfe;
    text-decoration: none;
    border-bottom: 1px dashed rgba(162, 155, 254, 0.5);
  }

  :global(.message-content a:hover) {
    border-bottom-style: solid;
  }
  
  :global(.message-content strong) { font-weight: 700; color: #fff; }
  :global(.message-content em) { font-style: italic; }
  :global(.message-content del) { text-decoration: line-through; opacity: 0.7; }
  
  .message-meta {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 6px;
    margin-top: 6px;
  }

  .message-time { font-size: 11px; color: var(--text-secondary); }
  .message-status { font-size: 12px; }
  .message-edited { font-size: 10px; color: var(--text-secondary); font-style: italic; }

  /* Message editing */
  .message-edit-container {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .message-edit-input {
    background: rgba(0, 0, 0, 0.2);
    border: 1px solid var(--bg-hover);
    border-radius: 8px;
    padding: 8px;
    color: var(--text-primary);
    font-size: 14px;
    resize: none;
    min-height: 40px;
    font-family: inherit;
  }

  .message-edit-input:focus {
    outline: none;
    border-color: var(--accent);
  }

  .message-edit-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .btn-sm {
    padding: 4px 12px;
    font-size: 12px;
    border-radius: 6px;
    cursor: pointer;
    border: none;
    transition: all 0.2s ease;
  }

  .btn-sm.btn-primary {
    background: var(--accent);
    color: white;
  }

  .btn-sm.btn-secondary {
    background: var(--bg-hover);
    color: var(--text-secondary);
  }

  .btn-sm:hover {
    opacity: 0.8;
    transform: scale(1.05);
  }

  .no-messages {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
  }

  .no-messages-icon { font-size: 64px; margin-bottom: 16px; opacity: 0.5; }

  .input-area-wrapper {
      display: flex;
      flex-direction: column;
      border-top: 1px solid var(--border);
      background: var(--bg-primary);
  }

  .input-area {
    padding: 12px 20px;
    display: flex;
    align-items: center;
    gap: 12px;
    position: relative;
  }
  
  .attachment-preview {
      width: 100%;
      background: var(--bg-secondary);
      padding: 10px 20px;
      display: flex;
      gap: 10px;
      overflow-x: auto;
      border-bottom: 1px solid var(--border);
      align-items: center;
  }
  
  .preview-item {
      position: relative;
      width: 60px;
      height: 60px;
      flex-shrink: 0;
  }
  
  .preview-img {
      width: 100%;
      height: 100%;
      object-fit: cover;
      border-radius: 8px;
      border: 1px solid var(--border);
  }
  
  .btn-remove-att {
      position: absolute;
      top: -5px;
      right: -5px;
      width: 18px;
      height: 18px;
      border-radius: 50%;
      background: var(--btn-danger);
      color: white;
      border: none;
      font-size: 10px;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
  }
  
  .compress-opt {
      margin-left: auto;
      font-size: 12px;
      color: var(--text-secondary);
      display: flex;
      align-items: center;
      gap: 6px;
  }

  .message-images {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
      gap: 4px;
      margin-bottom: 6px;
      border-radius: 8px;
      overflow: hidden;
  }
  
  .msg-img {
      width: 100%;
      height: 150px;
      object-fit: cover;
      cursor: pointer;
      background: rgba(0,0,0,0.2);
  }

  .message-input {
    flex: 1;
    padding: 14px 20px;
    border: none;
    border-radius: 24px;
    background: var(--bg-input);
    color: var(--text-primary);
    font-size: 14px;
    resize: none;
    outline: none;
    max-height: 120px;
    transition: box-shadow 0.3s;
  }

  .message-input:focus { box-shadow: 0 0 0 2px rgba(108, 92, 231, 0.3); }
  .message-input::placeholder { color: var(--text-secondary); }

  .btn-send {
    width: 48px;
    height: 48px;
    border: none;
    border-radius: 50%;
    background: linear-gradient(135deg, var(--accent), #5a4fcf);
    color: white;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.3s;
    flex-shrink: 0;
  }

  .btn-send:hover { transform: scale(1.1); box-shadow: 0 4px 15px rgba(108, 92, 231, 0.4); }
  .btn-send:disabled { opacity: 0.5; cursor: not-allowed; transform: none; }

  /* === No Chat === */
  .no-chat {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 16px;
    color: var(--text-secondary);
  }

  .no-chat-logo img { 
    width: 150px; 
    height: 150px; 
    opacity: 0.7; 
    border-radius: 50%; 
    object-fit: cover;
  }
  .no-chat h2 { color: var(--text-primary); font-size: 28px; }

  /* === Scrollbar === */
  ::-webkit-scrollbar { width: 6px; }
  ::-webkit-scrollbar-track { background: transparent; }
  ::-webkit-scrollbar-thumb { background: var(--bg-tertiary); border-radius: 3px; }
  ::-webkit-scrollbar-thumb:hover { background: var(--accent); }

  /* === Toast Notifications === */
  .toast-container {
    position: fixed;
    top: 20px;
    right: 20px;
    z-index: 10000;
    display: flex;
    flex-direction: column;
    gap: 10px;
    pointer-events: none;
  }

  .toast {
    padding: 12px 20px;
    border-radius: 8px;
    color: white;
    font-size: 14px;
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
    animation: toast-enter 0.3s ease-out, toast-exit 0.3s ease-in 2.7s forwards;
    pointer-events: auto;
    max-width: 320px;
  }

  .toast-success {
    background: linear-gradient(135deg, #4CAF50, #2E7D32);
  }

  .toast-error {
    background: linear-gradient(135deg, #F44336, #C62828);
  }

  .toast-info {
    background: linear-gradient(135deg, var(--accent), #5a4fcf);
  }

  @keyframes toast-enter {
    from {
      transform: translateX(100px);
      opacity: 0;
    }
    to {
      transform: translateX(0);
      opacity: 1;
    }
  }

  @keyframes toast-exit {
    from {
      transform: translateX(0);
      opacity: 1;
    }
    to {
      transform: translateX(100px);
      opacity: 0;
    }
  }

  /* Fullscreen Preview */
  .fullscreen-preview {
      position: fixed;
      top: 0; left: 0;
      width: 100vw; height: 100vh;
      background: rgba(0, 0, 0, 0.9);
      z-index: 9999;
      display: flex;
      align-items: center;
      justify-content: center;
      animation: fadeIn 0.3s ease;
  }

  .preview-content {
      position: relative;
      max-width: 90%;
      max-height: 90%;
  }

  .preview-content img {
      max-width: 100%;
      max-height: 90vh;
      border-radius: 8px;
      box-shadow: 0 0 20px rgba(0,0,0,0.5);
  }

  .close-preview {
      position: absolute;
      top: -40px;
      right: -40px;
      background: transparent;
      border: none;
      color: white;
      font-size: 32px;
      cursor: pointer;
      opacity: 0.7;
      transition: opacity 0.2s;
  }

  .close-preview:hover { opacity: 1; }

  /* === File Offer === */
  .file-offer-card {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 10px;
      background: rgba(0,0,0,0.2);
      border-radius: 8px;
      border: 1px solid rgba(255,255,255,0.05);
  }

  .file-icon-large {
      font-size: 32px;
      display: flex;
      align-items: center;
      justify-content: center;
  }

  .file-info {
      display: flex;
      flex-direction: column;
      flex: 1;
      min-width: 0;
  }

  .file-title {
      font-weight: 600;
      color: var(--text-primary);
      font-size: 14px;
      margin-bottom: 2px;
  }

  .file-size {
      font-size: 11px;
      color: var(--text-secondary);
  }
  
  .file-list-preview {
      color: var(--text-secondary);
      opacity: 0.7;
  }

  .file-actions {
      display: flex;
      gap: 8px;
      align-items: center;
      margin-top: 8px;
  }

  .btn-success {
      background: var(--accent); /* Use accent or specific green? */
      background: #00b894;
      color: white;
      border: none;
      padding: 6px 12px;
      border-radius: 6px;
      cursor: pointer;
      font-weight: 500;
      font-size: 12px;
      border: 1px solid rgba(0,0,0,0.1);
  }
  
  .btn-success:hover { background: #00a884; }

  .status-badge {
      font-size: 11px;
      color: var(--text-secondary);
      background: rgba(0,0,0,0.2);
      padding: 4px 8px;
      border-radius: 4px;
      font-style: italic;
      display: inline-block;
  }
</style>

<svelte:window on:paste={handlePaste} />

<!-- Full Screen Image Preview -->
{#if previewImage}
<div class="fullscreen-preview" on:click={() => previewImage = null}>
    <div class="preview-content" on:click|stopPropagation>
        <img src={previewImage} alt="Full preview" />
        <button class="close-preview" on:click={() => previewImage = null}>✕</button>
    </div>
</div>
{/if}

<!-- Custom Confirm Modal -->
{#if showConfirmModal}
<div class="modal-backdrop animate-fade-in" on:click|self={closeConfirmModal}>
    <div class="modal-content animate-slide-down" style="max-width: 400px;">
        <div class="modal-header">
            <h3>{confirmModalTitle}</h3>
        </div>
        <div class="modal-body">
            <p style="color: var(--text-secondary); margin-bottom: 20px;">{confirmModalText}</p>
        </div>
        <div class="modal-footer">
            <button class="btn-small btn-secondary" on:click={closeConfirmModal}>Отмена</button>
            <button class="btn-small btn-primary" on:click={handleConfirmAction}>OK</button>
        </div>
    </div>
</div>
{/if}

<!-- Context Menu for Contacts -->
{#if contextMenu.show}
<div class="context-menu" style="top: {contextMenu.y}px; left: {contextMenu.x}px">
    <div class="context-item" on:click={copyAddressFromMenu}>Скопировать адрес</div>
    <div class="context-item" style="color: #ff6b6b;" on:click={deleteContactFromMenu}>Удалить контакт</div>
</div>
{/if}

<!-- Context Menu for Messages/Files -->
{#if messageContextMenu.show}
<div class="context-menu" style="top: {messageContextMenu.y}px; left: {messageContextMenu.x}px">
    {#if messageContextMenu.imagePath}
        <div class="context-item" on:click={() => copyImageToClipboard(messageContextMenu.imagePath)}>Скопировать картинку</div>
        <div class="context-item" on:click={() => showInFolder(messageContextMenu.imagePath)}>Показать в папке</div>
    {:else if messageContextMenu.filePath}
        <div class="context-item" on:click={() => openFile(messageContextMenu.filePath)}>Открыть файл</div>
        <div class="context-item" on:click={() => copyPathToClipboard(messageContextMenu.filePath)}>Скопировать путь</div>
        <div class="context-item" on:click={() => showInFolder(messageContextMenu.filePath)}>Показать в папке</div>
    {:else}
        {#if messageContextMenu.message && !messageContextMenu.message.isOutgoing}
             <!-- Incoming message options -->
             <div class="context-item" on:click={() => { navigator.clipboard.writeText(messageContextMenu.message.content); messageContextMenu.show=false; }}>Скопировать текст</div>
        {:else}
             <!-- Outgoing message options -->
             <div class="context-item" on:click={() => startEditMessage(messageContextMenu.message)}>Редактировать</div>
             <div class="context-item" on:click={() => { navigator.clipboard.writeText(messageContextMenu.message.content); messageContextMenu.show=false; }}>Скопировать текст</div>
        {/if}
    {/if}
    
    {#if messageContextMenu.message}
        <div class="context-item" style="color: #ff6b6b;" on:click={() => deleteMsg(messageContextMenu.message)}>Удалить у себя</div>
        <div class="context-item" style="color: #ff6b6b;" on:click={() => deleteMsg(messageContextMenu.message, true)}>Удалить у всех</div>
    {/if}
</div>
{/if}
