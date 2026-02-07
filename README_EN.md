# TeleGhost 👻 ![visitors](https://visitor-badge.laobi.icu/badge?page_id=kiktor12358.TeleGhost)

[Russian version / Русская версия](README.md)

---

**TeleGhost** is a modern, fast, and anonymous messenger operating within the I2P network. It provides a high degree of privacy and secure communication using end-to-end encryption and hidden network tunnels.

### ✨ Features

- **Out-of-the-box Anonymity**: All traffic goes through the embedded **i2pd** node, hiding your real IP address.
- **End-to-End Encryption (E2EE)**: Your messages can only be read by you and your recipient.
- **Chat Folders**: Organize your contacts exactly how you want. Now with custom emoji support!
- **Avatars & Profiles**: Personalize your account; your data syncs with contacts via I2P (in real-time).
- **Fast Search**: Find the right chats and messages instantly.
- **Premium UI**: Modern design with dark mode and smooth animations.

## 🗺 Roadmap

### Phase 1: Rich Media (In Progress)
- **Voice Messages**: Opus compression + chunked delivery for I2P stability.
- **Files & Photos**: On-client compression and Resume capability for file transfers.
- **Local Security**: Full SQLite database encryption using a key derived from your Seed phrase.

### Phase 2: GhostMail & Federation
- **Offline Delivery**: Hybrid P2P + Home Server (Store-and-Forward) architecture.
- **Server Federation**: Encrypted mail exchange between trusted nodes.
- **Anti-Spam**: Proof-of-Work (RandomX/SHA) implementation for unknown senders.

### Phase 3: Real-Time & Mobility
- **Calls**: Audio calls via UDP (SSU2) support.
- **Security Profiles**: On-the-fly tunnel mode switching (🚀 **Fast**, 🛡️ **Default**, 👻 **Invisible**).
- **Mobile Support**: Engine optimization for mobile platforms and development of a native mobile client.


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
