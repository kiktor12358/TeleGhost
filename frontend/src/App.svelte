<script>
  import { onMount } from 'svelte';
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
    CopyToClipboard,
    UpdateMyProfile
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

  // Экраны
  let screen = 'login'; // login | main | settings
  let showSettings = false;
  let showAddContact = false;
  
  // Настройки профиля
  let profileNickname = 'User';
  let profileBio = '';
  let addContactName = '';
  let addContactAddress = '';

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
    await waitForWails();
    if (!wailsReady) return;

    try {
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
    if (!wailsReady) { alert('Приложение не готово'); return; }
    
    isLoading = true;
    try {
      await Login(seedPhrase);
      screen = 'main';
      await loadInitialData();
    } catch (e) {
      alert('Ошибка: ' + e);
    }
    isLoading = false;
  }

  // Mnemonic Modal State
  let showMnemonicModal = false;
  let newMnemonic = '';

  async function handleCreateAccount() {
    if (!wailsReady) await waitForWails();
    if (!wailsReady) { alert('Приложение не готово'); return; }
    
    isLoading = true;
    try {
      const mnemonic = await CreateAccount();
      if (mnemonic) {
        newMnemonic = mnemonic;
        showMnemonicModal = true;
      }
    } catch (e) {
      alert('Ошибка: ' + e);
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
      }
      await loadContacts();
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
        alert('✅ Контакт добавлен: ' + contact.nickname);
      }
    } catch (e) {
      alert('❌ ' + e);
    }
  }

  async function handleAddContactManual() {
    if (!addContactName.trim() || !addContactAddress.trim()) return;
    try {
      await AddContact(addContactName, addContactAddress);
      await loadContacts();
      showAddContact = false;
      addContactName = '';
      addContactAddress = '';
      alert('✅ Контакт добавлен');
    } catch (e) {
      alert('❌ ' + e);
    }
  }

  // === Messages ===
  async function sendMessage() {
    if (!newMessage.trim() || !selectedContact) return;
    const text = newMessage;
    newMessage = '';
    
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
      alert('❌ ' + e);
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
      await UpdateMyProfile(profileNickname, profileBio);
      alert('✅ Профиль сохранён');
    } catch (e) {
      alert('❌ ' + e);
    }
  }

  function copyDestination() {
    CopyToClipboard(myDestination);
    alert('📋 Скопировано!');
  }

  function scrollToBottom() {
    const chat = document.querySelector('.messages-container');
    if (chat) chat.scrollTop = chat.scrollHeight;
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
</script>

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
         <button class="btn-text" on:click={() => { CopyToClipboard(newMnemonic); alert('Скопировано!'); }}>
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
  <div class="sidebar">
    <div class="sidebar-header">
      <div class="sidebar-logo" on:click={() => { showSettings = true; selectedContact = null; }}>
        <img src={logo} alt="TG" class="sidebar-logo-img" />
        <span>TeleGhost</span>
      </div>
      <div class="sidebar-actions">
        <button class="btn-icon" on:click={() => showAddContact = !showAddContact} title="Добавить">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor">
            <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/>
          </svg>
        </button>
        <button class="btn-icon" on:click={() => { showSettings = true; selectedContact = null; }} title="Настройки">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor">
            <path d="M19.14 12.94c.04-.31.06-.63.06-.94 0-.31-.02-.63-.06-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.04.31-.06.63-.06.94s.02.63.06.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/>
          </svg>
        </button>
      </div>
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
      {#each contacts as contact (contact.id)}
        <div 
          class="contact-item" 
          class:selected={selectedContact?.id === contact.id}
          on:click={() => selectContact(contact)}
          on:keypress={(e) => e.key === 'Enter' && selectContact(contact)}
          tabindex="0"
          role="button"
        >
          <div class="contact-avatar" style="background: linear-gradient(135deg, hsl({contact.id.charCodeAt(0) * 10}, 70%, 50%), hsl({contact.id.charCodeAt(1) * 10}, 70%, 40%))">
            {getInitials(contact.nickname)}
          </div>
          <div class="contact-info">
            <div class="contact-name">{contact.nickname}</div>
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
  
  <!-- Main Content -->
  <div class="content-area">
    {#if showSettings}
      <!-- Settings Panel -->
      <div class="settings-panel animate-fade-in">
        <div class="settings-header">
          <h2>⚙️ Настройки профиля</h2>
        </div>
        
        <div class="settings-content">
          <div class="profile-section">
            <div class="profile-avatar-large">
              <img src={logo} alt="Avatar" class="avatar-img" />
              <button class="avatar-edit-btn">📷</button>
            </div>
            
            <div class="profile-form">
              <label class="form-label">
                Никнейм
                <input type="text" bind:value={profileNickname} class="input-field" placeholder="Ваш никнейм" />
              </label>
              
              <label class="form-label">
                О себе
                <textarea bind:value={profileBio} class="input-field" rows="3" placeholder="Расскажите о себе..."></textarea>
              </label>
              
              <button class="btn-primary" on:click={saveProfile}>💾 Сохранить профиль</button>
            </div>
          </div>
          
          <div class="settings-section">
            <h3>🔑 Мой I2P адрес</h3>
            <div class="destination-box">
              <code class="destination-code">{myDestination ? myDestination.slice(0, 64) + '...' : 'Загрузка...'}</code>
              <button class="btn-icon-small" on:click={copyDestination}>📋</button>
            </div>
            <p class="hint">Поделитесь этим адресом, чтобы вас могли найти</p>
          </div>
          
          <div class="settings-section">
            <h3>ℹ️ Информация</h3>
            <div class="info-grid">
              <div class="info-item">
                <span class="info-label">Статус сети</span>
                <span class="info-value" style="color: {getStatusColor(networkStatus)}">{getStatusText(networkStatus)}</span>
              </div>
              <div class="info-item">
                <span class="info-label">Версия</span>
                <span class="info-value">1.0.0-alpha</span>
              </div>
            </div>
          </div>
        </div>
      </div>

    {:else if selectedContact}
      <!-- Chat -->
      <div class="chat-area animate-fade-in">
        <div class="chat-header">
          <div class="chat-contact-info">
            <div class="chat-avatar" style="background: linear-gradient(135deg, hsl({selectedContact.id.charCodeAt(0) * 10}, 70%, 50%), hsl({selectedContact.id.charCodeAt(1) * 10}, 70%, 40%))">
              {getInitials(selectedContact.nickname)}
            </div>
            <div>
              <div class="chat-name">{selectedContact.nickname}</div>
              <div class="chat-status">{selectedContact.i2pAddress}</div>
            </div>
          </div>
        </div>
        
        <div class="messages-container">
          {#each messages as msg (msg.id)}
            <div class="message animate-message" class:outgoing={msg.isOutgoing}>
              <div class="message-bubble" class:outgoing={msg.isOutgoing}>
                <div class="message-content">{msg.content}</div>
                <div class="message-meta">
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
  .chat-area { flex: 1; display: flex; flex-direction: column; }

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

  .chat-name { color: var(--text-primary); font-weight: 600; font-size: 16px; }
  .chat-status { color: var(--text-secondary); font-size: 12px; }

  .messages-container {
    flex: 1;
    overflow-y: auto;
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 10px;
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

  .message-content { color: var(--text-primary); line-height: 1.5; word-wrap: break-word; }

  .message-meta {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 6px;
    margin-top: 6px;
  }

  .message-time { font-size: 11px; color: var(--text-secondary); }
  .message-status { font-size: 12px; }

  .no-messages {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
  }

  .no-messages-icon { font-size: 64px; margin-bottom: 16px; opacity: 0.5; }

  .input-area {
    padding: 16px 20px;
    display: flex;
    align-items: flex-end;
    gap: 14px;
    background: var(--bg-secondary);
    border-top: 1px solid var(--border);
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
</style>
