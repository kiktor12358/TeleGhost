# TeleGhost 👻 ![visitors](https://visitor-badge.laobi.icu/badge?page_id=kiktor12358.TeleGhost)

[Russian version / Русская версия](README.md)

---

**TeleGhost** is a modern, anonymous, and fully decentralized alternative to Telegram. It is a powerful messenger operating within the hidden I2P network, now implemented and fully functional on **Android**! It provides the highest degree of privacy and reliability, unavailable in regular messengers, using end-to-end encryption and hidden network tunnels. The perfect Telegram analog for those who value absolute security and control over their data.

### ✨ Features

- **Out-of-the-box Anonymity**: All traffic goes through the embedded **i2pd** node, hiding your real IP address.
- **End-to-End Encryption (E2EE)**: Your messages can only be read by you and your recipient.
- **Chat Folders**: Organize your contacts exactly how you want. Now with custom emoji support!
- **Avatars & Profiles**: Personalize your account; your data syncs with contacts via I2P (in real-time).
- **Fast Search**: Find the right chats and messages instantly.
- **Premium UI**: Modern design with dark mode and smooth animations.

## 📸 Screenshots

<p align="center">
  <img src="assets/login_screen.png" alt="PC Login Screen" width="45%">
  &nbsp;
  <img src="assets/main_screen.png" alt="PC Main Screen" width="45%">
</p>

### 📱 Android Screenshots

<p align="center">
  <img src="assets/login_screen.png" alt="Main Screen" width="45%">
  &nbsp;
  <img src="assets/main_screen.png" alt="Login Screen" width="45%">
</p>

## 🚀 Quick Start

### Download

You can download the latest ready-to-use versions of TeleGhost for your system from the [Releases](https://github.com/kiktor12358/TeleGhost/releases/latest) page:

*   **Android**: Download `teleghost.aar` or `app-release.apk`, install it on your Android smartphone, and enjoy secure communication.
*   **Windows**: Download `TeleGhost.exe`, run it, and the messenger is ready to work (includes embedded i2pd router).
*   **Linux**: Download `TeleGhost-linux-amd64`, make the file executable (`chmod +x TeleGhost-linux-amd64`), and run it.

---

## 🗺 Roadmap

### Rich Media (In Progress)
- **Voice Messages**: Opus compression + chunked delivery for I2P stability.
- [**Implemented**] ~~**Files & Photos**: On-client compression and Resume capability for file transfers.~~
- [**Implemented**] ~~**Local Security**: Full SQLite database encryption using a key derived from your Seed phrase.~~

### GhostMail & Federation
- **Offline Delivery**: Hybrid P2P + Home Server (Store-and-Forward) architecture.
- **Server Federation**: Encrypted mail exchange between trusted nodes.
- **Anti-Spam**: Proof-of-Work (RandomX/SHA) implementation for unknown senders.

### Real-Time & Mobility
- **Calls**: Audio calls via UDP (SSU2) support.
- [**Implemented**] ~~**Security Profiles**: On-the-fly tunnel mode switching (🚀 **Fast**, 🛡️ **Default**, 👻 **Invisible**).~~
- [**Implemented**] ~~**Mobile Support**: Engine optimization for mobile platforms and development of a native mobile client.~~


---

## 🚀 Technologies

- **Backend**: Go (Golang)
- **Frontend**: Svelte, Vite
- **Network**: I2P (i2pd) via SAM bridge
- **Database**: SQLite3
- **Framework**: [Wails v2](https://wails.io)

## 🛠 Installation

### Requirements
- Go 1.21+
- Node.js & npm
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Development
```bash
wails dev
```

### Build
```bash
wails build -tags cgo_i2pd
```

## 🔐 Security
TeleGhost does not use centralized servers. All data is stored locally on your device, and transmission occurs directly between I2P nodes.

## 📄 License
Distributed under the MIT License. See `LICENSE` for more information.

---
*Developed with privacy in mind.*
