# TeleGhost 👻 ![visitors](https://visitor-badge.laobi.icu/badge?page_id=kiktor12358.TeleGhost)

[RU] [Русский](#русский) | [EN] [English](#english)

---

## <a name="русский"></a> 🇷🇺 Русский

**TeleGhost** — это современный, быстрый и анонимный мессенджер, работающий внутри сети I2P. Он обеспечивает высокую степень приватности и безопасности общения, используя сквозное шифрование и скрытые сетевые туннели.

### ✨ Особенности

- **Анонимность из коробки**: Весь трафик идет через сеть I2P, скрывая ваш реальный IP-адрес.
- **Сквозное шифрование (E2EE)**: Ваши сообщения могут быть прочитаны только вами и вашим собеседником.
- **Папки для чатов**: Организуйте свои контакты так, как вам удобно. Теперь с поддержкой эмодзи-иконок!
- **Аватарки и Профили**: Персонализируйте свой аккаунт, ваши данные синхронизируются с контактами через I2P.
- **Быстрый поиск**: Находите нужные чаты и сообщения мгновенно.
- **Премиальный UI**: Современный дизайн с поддержкой темных тем и плавных анимаций.

---

## <a name="english"></a> 🇺🇸 English

**TeleGhost** is a modern, fast, and anonymous messenger operating within the I2P network. It provides a high degree of privacy and secure communication using end-to-end encryption and hidden network tunnels.

### ✨ Features

- **Out-of-the-box Anonymity**: All traffic goes through the I2P network, hiding your real IP address.
- **End-to-End Encryption (E2EE)**: Your messages can only be read by you and your recipient.
- **Chat Folders**: Organize your contacts exactly how you want. Now with custom emoji support!
- **Avatars & Profiles**: Personalize your account; your data syncs with contacts via I2P.
- **Fast Search**: Find the right chats and messages instantly.
- **Premium UI**: Modern design with dark mode and smooth animations.

---

## 🚀 Технологии / Technologies

- **Backend**: Go (Golang)
- **Frontend**: Svelte, Vite
- **Network**: I2P (i2pd) via SAM bridge
- **Database**: SQLite3
- **Framework**: [Wails v2](https://wails.io)

## 🛠 Установка и запуск / Installation

### Требования / Requirements
- Go 1.21+
- Node.js & npm
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Запуск / Development
```bash
wails dev
```

### Сборка / Build
```bash
wails build -tags cgo_i2pd
```

## 🔐 Безопасность / Security
TeleGhost не использует централизованные серверы. Все данные хранятся локально на вашем устройстве, а передача осуществляется напрямую между I2P-узлами.

TeleGhost does not use centralized servers. All data is stored locally on your device, and transmission occurs directly between I2P nodes.

## 📄 Лицензия / License
Distributed under the MIT License. See `LICENSE` for more information.

---
*Разработано с заботой о приватности / Developed with privacy in mind.*
