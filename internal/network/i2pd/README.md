# i2pd CGO Integration for TeleGhost

Этот модуль предоставляет CGO биндинги к **i2pd** — полноценному C++ I2P роутеру.

## Требования

### Системные пакеты (Debian/Ubuntu)

```bash
sudo apt update
sudo apt install -y \
    build-essential \
    cmake \
    libboost-all-dev \
    libssl-dev \
    zlib1g-dev \
    git
```

### Arch Linux

```bash
sudo pacman -S base-devel cmake boost openssl zlib git
```

## Сборка

### Автоматическая

```bash
cd internal/network/i2pd
chmod +x build.sh
./build.sh
```

### Ручная

```bash
# 1. Клонируем i2pd
git clone --depth 1 https://github.com/PurpleI2P/i2pd.git

# 2. Собираем libi2pd
cd i2pd
mkdir build && cd build
cmake -DWITH_STATIC=ON -DWITH_LIBRARY=ON -DWITH_BINARY=OFF ..
make -j$(nproc) libi2pd

# 3. Собираем wrapper
cd ../../
g++ -std=c++17 -c i2pd_wrapper.cpp \
    -Ii2pd/libi2pd -Ii2pd/libi2pd_client -Ii2pd/i18n -Ii2pd
ar rcs libi2pd_wrapper.a i2pd_wrapper.o
```

## Использование

```go
import "teleghost/internal/network/i2pd"

// Создаём роутер
router := i2pd.NewRouter(&i2pd.Config{
    DataDir:    "~/.teleghost/i2pd",
    SAMEnabled: true,
    SAMPort:    7656,
})

// Запускаем
err := router.Start(ctx)

// Проверяем готовность
if router.IsRunning() {
    addr := router.GetSAMAddress()  // "127.0.0.1:7656"
}

// Останавливаем
router.Stop()
```

## Что включено

- **NTCP2** — транспорт для NAT traversal
- **SSU2** — UDP транспорт для скорости
- **SAM API** — для подключения приложения

## Что отключено

- HTTP Console
- SOCKS/HTTP Proxy
- BOB, I2CP, I2PControl
- Floodfill (клиентский режим)

## Первый запуск

Первый запуск i2pd занимает **5-10 минут** для:
1. Генерации ключей роутера
2. Bootstrap через reseed серверы
3. Построения туннелей

После первого запуска данные сохраняются в `DataDir`.
