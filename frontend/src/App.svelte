<script>
  import { onMount, onDestroy } from 'svelte';
  import { EventsOn } from '../wailsjs/runtime/runtime.js';
  import { 
    Login, 
    CreateAccount,
    GetNetworkStatus,
    GetMyDestination,
    GetContacts,
    GetMessages,
    SendText,
    AddContactFromClipboard,
    CopyToClipboard
  } from '../wailsjs/go/main/App.js';

  // Состояние
  let isLoggedIn = false;
  let isLoading = false;
  let seedPhrase = '';
  let networkStatus = 'offline';
  let myDestination = '';
  
  // Контакты и сообщения
  let contacts = [];
  let selectedContact = null;
  let messages = [];
  let newMessage = '';

  // Экран (login | main)
  let screen = 'login';

  onMount(() => {
    // Подписываемся на события
    EventsOn('network_status', (status) => {
      networkStatus = status;
    });

    EventsOn('new_message', (msg) => {
      if (selectedContact && msg.chatId === selectedContact.chatId) {
        messages = [...messages, msg];
        scrollToBottom();
      }
      loadContacts();
    });

    GetNetworkStatus().then(s => networkStatus = s);
  });

  // === Авторизация ===
  async function handleLogin() {
    if (!seedPhrase.trim()) return;
    isLoading = true;
    try {
      await Login(seedPhrase);
      isLoggedIn = true;
      screen = 'main';
      await loadInitialData();
    } catch (e) {
      alert('Ошибка: ' + e);
    }
    isLoading = false;
  }

  async function handleCreateAccount() {
    isLoading = true;
    try {
      const mnemonic = await CreateAccount();
      seedPhrase = mnemonic;
      alert('Ваша seed-фраза (сохраните её!):\n\n' + mnemonic);
      isLoggedIn = true;
      screen = 'main';
      await loadInitialData();
    } catch (e) {
      alert('Ошибка: ' + e);
    }
    isLoading = false;
  }

  async function loadInitialData() {
    myDestination = await GetMyDestination();
    await loadContacts();
  }

  async function loadContacts() {
    try {
      contacts = await GetContacts() || [];
    } catch (e) {
      console.error('Failed to load contacts:', e);
    }
  }

  // === Контакты ===
  async function selectContact(contact) {
    selectedContact = contact;
    await loadMessages();
  }

  async function loadMessages() {
    if (!selectedContact) return;
    try {
      messages = await GetMessages(selectedContact.id, 100, 0) || [];
      messages = messages.reverse(); // Для отображения снизу вверх
      setTimeout(scrollToBottom, 100);
    } catch (e) {
      console.error('Failed to load messages:', e);
    }
  }

  async function handleAddContact() {
    try {
      const contact = await AddContactFromClipboard();
      if (contact) {
        await loadContacts();
        alert('Контакт добавлен: ' + contact.nickname);
      }
    } catch (e) {
      alert('Ошибка: ' + e);
    }
  }

  // === Сообщения ===
  async function sendMessage() {
    if (!newMessage.trim() || !selectedContact) return;
    const text = newMessage;
    newMessage = '';
    
    // Оптимистичное добавление
    messages = [...messages, {
      id: Date.now().toString(),
      content: text,
      timestamp: Date.now(),
      isOutgoing: true,
      status: 'sending'
    }];
    scrollToBottom();

    try {
      await SendText(selectedContact.id, text);
    } catch (e) {
      alert('Ошибка отправки: ' + e);
    }
  }

  function handleKeyPress(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  }

  function copyDestination() {
    CopyToClipboard(myDestination);
    alert('Скопировано!');
  }

  function scrollToBottom() {
    const chat = document.querySelector('.messages-container');
    if (chat) chat.scrollTop = chat.scrollHeight;
  }

  function formatTime(timestamp) {
    const d = new Date(timestamp);
    return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
  }

  function getInitials(name) {
    return name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2);
  }

  function getStatusColor(status) {
    switch(status) {
      case 'online': return '#4CAF50';
      case 'connecting': return '#FFC107';
      case 'error': return '#F44336';
      default: return '#9E9E9E';
    }
  }

  function getStatusText(status) {
    switch(status) {
      case 'online': return 'Онлайн';
      case 'connecting': return 'Подключение к I2P...';
      case 'error': return 'Ошибка сети';
      default: return 'Оффлайн';
    }
  }
</script>

<!-- Login Screen -->
{#if screen === 'login'}
<div class="login-screen">
  <div class="login-container">
    <div class="login-logo">
      <svg viewBox="0 0 64 64" width="80" height="80">
        <circle cx="32" cy="32" r="30" fill="#5865F2"/>
        <path d="M20 32 L28 40 L44 24" stroke="white" stroke-width="4" fill="none" stroke-linecap="round"/>
      </svg>
    </div>
    <h1 class="login-title">TeleGhost</h1>
    <p class="login-subtitle">Децентрализованный мессенджер на I2P</p>
    
    <div class="login-form">
      <textarea
        class="seed-input"
        placeholder="Введите вашу seed-фразу (12 слов)..."
        bind:value={seedPhrase}
        rows="3"
      ></textarea>
      
      <button class="btn-primary" on:click={handleLogin} disabled={isLoading}>
        {isLoading ? 'Загрузка...' : 'Войти'}
      </button>
      
      <div class="divider">
        <span>или</span>
      </div>
      
      <button class="btn-secondary" on:click={handleCreateAccount} disabled={isLoading}>
        Создать новый аккаунт
      </button>
    </div>
  </div>
</div>

<!-- Main Screen -->
{:else}
<div class="main-screen">
  <!-- Sidebar -->
  <div class="sidebar">
    <div class="sidebar-header">
      <div class="sidebar-title">TeleGhost</div>
      <button class="btn-icon" on:click={handleAddContact} title="Добавить контакт">
        <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
          <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/>
        </svg>
      </button>
    </div>
    
    <!-- Network Status -->
    <div class="network-status" style="background: {getStatusColor(networkStatus)}20">
      <div class="status-dot" style="background: {getStatusColor(networkStatus)}"></div>
      <span>{getStatusText(networkStatus)}</span>
    </div>
    
    <!-- Contacts List -->
    <div class="contacts-list">
      {#each contacts as contact}
        <div 
          class="contact-item" 
          class:selected={selectedContact?.id === contact.id}
          on:click={() => selectContact(contact)}
        >
          <div class="contact-avatar" style="background: hsl({contact.id.charCodeAt(0) * 10}, 70%, 50%)">
            {getInitials(contact.nickname || 'NC')}
          </div>
          <div class="contact-info">
            <div class="contact-name">{contact.nickname}</div>
            <div class="contact-last-message">{contact.lastMessage || 'Нет сообщений'}</div>
          </div>
        </div>
      {/each}
      
      {#if contacts.length === 0}
        <div class="no-contacts">
          <p>Нет контактов</p>
          <p class="hint">Нажмите + чтобы добавить</p>
        </div>
      {/if}
    </div>
    
    <!-- My Destination -->
    <div class="my-destination">
      <button class="btn-copy" on:click={copyDestination} title="Копировать I2P адрес">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
          <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
        </svg>
        <span>Мой I2P адрес</span>
      </button>
    </div>
  </div>
  
  <!-- Chat Area -->
  <div class="chat-area">
    {#if selectedContact}
      <!-- Chat Header -->
      <div class="chat-header">
        <div class="chat-contact-info">
          <div class="chat-avatar" style="background: hsl({selectedContact.id.charCodeAt(0) * 10}, 70%, 50%)">
            {getInitials(selectedContact.nickname || 'NC')}
          </div>
          <div>
            <div class="chat-name">{selectedContact.nickname}</div>
            <div class="chat-status">{selectedContact.i2pAddress}</div>
          </div>
        </div>
      </div>
      
      <!-- Messages -->
      <div class="messages-container">
        {#each messages as msg}
          <div class="message" class:outgoing={msg.isOutgoing}>
            <div class="message-bubble" class:outgoing={msg.isOutgoing}>
              <div class="message-content">{msg.content}</div>
              <div class="message-time">{formatTime(msg.timestamp)}</div>
            </div>
          </div>
        {/each}
        
        {#if messages.length === 0}
          <div class="no-messages">
            <p>Начните переписку!</p>
          </div>
        {/if}
      </div>
      
      <!-- Input Area -->
      <div class="input-area">
        <textarea
          class="message-input"
          placeholder="Сообщение..."
          bind:value={newMessage}
          on:keypress={handleKeyPress}
          rows="1"
        ></textarea>
        <button class="btn-send" on:click={sendMessage} disabled={!newMessage.trim()}>
          <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
            <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/>
          </svg>
        </button>
      </div>
    {:else}
      <!-- No Chat Selected -->
      <div class="no-chat-selected">
        <svg viewBox="0 0 64 64" width="120" height="120" fill="#5865F233">
          <circle cx="32" cy="32" r="30"/>
          <path d="M20 32 L28 40 L44 24" stroke="#5865F2" stroke-width="4" fill="none"/>
        </svg>
        <p>Выберите чат, чтобы начать общение</p>
      </div>
    {/if}
  </div>
</div>
{/if}

<style>
  /* === Variables === */
  :root {
    --bg-primary: #17212b;
    --bg-secondary: #0e1621;
    --bg-sidebar: #17212b;
    --bg-chat: #0e1621;
    --bg-input: #242f3d;
    --bg-message-out: #2b5278;
    --bg-message-in: #182533;
    --text-primary: #ffffff;
    --text-secondary: #8b9ba5;
    --accent-color: #5865F2;
    --border-color: #0e1621;
  }

  /* === Reset === */
  * {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
  }

  /* === Login Screen === */
  .login-screen {
    width: 100vw;
    height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(135deg, var(--bg-primary) 0%, #1e2a38 100%);
  }

  .login-container {
    width: 100%;
    max-width: 400px;
    padding: 40px;
    text-align: center;
  }

  .login-logo {
    margin-bottom: 24px;
  }

  .login-title {
    font-size: 32px;
    font-weight: 700;
    color: var(--text-primary);
    margin-bottom: 8px;
  }

  .login-subtitle {
    color: var(--text-secondary);
    margin-bottom: 32px;
  }

  .login-form {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .seed-input {
    width: 100%;
    padding: 16px;
    border: none;
    border-radius: 12px;
    background: var(--bg-input);
    color: var(--text-primary);
    font-size: 14px;
    resize: none;
    outline: none;
  }

  .seed-input::placeholder {
    color: var(--text-secondary);
  }

  .btn-primary {
    width: 100%;
    padding: 16px;
    border: none;
    border-radius: 12px;
    background: var(--accent-color);
    color: white;
    font-size: 16px;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.2s;
  }

  .btn-primary:hover {
    opacity: 0.9;
  }

  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-secondary {
    width: 100%;
    padding: 16px;
    border: 1px solid var(--accent-color);
    border-radius: 12px;
    background: transparent;
    color: var(--accent-color);
    font-size: 16px;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.2s;
  }

  .btn-secondary:hover {
    background: var(--accent-color)22;
  }

  .divider {
    display: flex;
    align-items: center;
    gap: 12px;
    color: var(--text-secondary);
  }

  .divider::before,
  .divider::after {
    content: '';
    flex: 1;
    height: 1px;
    background: var(--border-color);
  }

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
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border-color);
    display: flex;
    flex-direction: column;
  }

  .sidebar-header {
    padding: 16px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border-color);
  }

  .sidebar-title {
    font-size: 20px;
    font-weight: 700;
    color: var(--text-primary);
  }

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
    transition: background 0.2s;
  }

  .btn-icon:hover {
    background: var(--bg-input);
    color: var(--text-primary);
  }

  .network-status {
    padding: 8px 16px;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .contacts-list {
    flex: 1;
    overflow-y: auto;
  }

  .contact-item {
    padding: 12px 16px;
    display: flex;
    align-items: center;
    gap: 12px;
    cursor: pointer;
    transition: background 0.2s;
  }

  .contact-item:hover {
    background: var(--bg-input);
  }

  .contact-item.selected {
    background: var(--accent-color)33;
  }

  .contact-avatar {
    width: 48px;
    height: 48px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
    font-weight: 600;
    font-size: 16px;
  }

  .contact-info {
    flex: 1;
    min-width: 0;
  }

  .contact-name {
    color: var(--text-primary);
    font-weight: 500;
    margin-bottom: 4px;
  }

  .contact-last-message {
    color: var(--text-secondary);
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .no-contacts {
    padding: 40px;
    text-align: center;
    color: var(--text-secondary);
  }

  .no-contacts .hint {
    font-size: 12px;
    margin-top: 8px;
  }

  .my-destination {
    padding: 12px;
    border-top: 1px solid var(--border-color);
  }

  .btn-copy {
    width: 100%;
    padding: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    border: 1px dashed var(--text-secondary);
    border-radius: 8px;
    background: transparent;
    color: var(--text-secondary);
    font-size: 13px;
    cursor: pointer;
    transition: all 0.2s;
  }

  .btn-copy:hover {
    border-color: var(--accent-color);
    color: var(--accent-color);
  }

  /* === Chat Area === */
  .chat-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    background: var(--bg-chat);
  }

  .chat-header {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border-color);
    background: var(--bg-sidebar);
  }

  .chat-contact-info {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .chat-avatar {
    width: 42px;
    height: 42px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
    font-weight: 600;
  }

  .chat-name {
    color: var(--text-primary);
    font-weight: 600;
  }

  .chat-status {
    color: var(--text-secondary);
    font-size: 12px;
  }

  .messages-container {
    flex: 1;
    overflow-y: auto;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .message {
    display: flex;
    justify-content: flex-start;
  }

  .message.outgoing {
    justify-content: flex-end;
  }

  .message-bubble {
    max-width: 65%;
    padding: 10px 14px;
    border-radius: 18px;
    background: var(--bg-message-in);
    border-bottom-left-radius: 4px;
  }

  .message-bubble.outgoing {
    background: var(--bg-message-out);
    border-bottom-left-radius: 18px;
    border-bottom-right-radius: 4px;
  }

  .message-content {
    color: var(--text-primary);
    word-wrap: break-word;
    line-height: 1.4;
  }

  .message-time {
    font-size: 11px;
    color: var(--text-secondary);
    text-align: right;
    margin-top: 4px;
  }

  .no-messages {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
  }

  .input-area {
    padding: 12px 16px;
    display: flex;
    align-items: flex-end;
    gap: 12px;
    background: var(--bg-sidebar);
    border-top: 1px solid var(--border-color);
  }

  .message-input {
    flex: 1;
    padding: 12px 16px;
    border: none;
    border-radius: 20px;
    background: var(--bg-input);
    color: var(--text-primary);
    font-size: 14px;
    resize: none;
    outline: none;
    max-height: 120px;
  }

  .message-input::placeholder {
    color: var(--text-secondary);
  }

  .btn-send {
    width: 44px;
    height: 44px;
    border: none;
    border-radius: 50%;
    background: var(--accent-color);
    color: white;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: opacity 0.2s;
  }

  .btn-send:hover {
    opacity: 0.9;
  }

  .btn-send:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .no-chat-selected {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 16px;
    color: var(--text-secondary);
  }
</style>
