#!/bin/bash
# Windows (MSYS2) Build script for i2pd integration with TeleGhost
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
I2PD_DIR="$SCRIPT_DIR/i2pd"

echo "=== TeleGhost i2pd Windows (MSYS2) Build Script ==="

# Clone i2pd if not exists or is incomplete
if [ ! -d "$I2PD_DIR/.git" ] || [ ! -f "$I2PD_DIR/build/CMakeLists.txt" ]; then
    echo "i2pd directory is missing, incomplete, or not a git repo. Cleaning and cloning fresh..."
    rm -rf "$I2PD_DIR"
    git clone --depth 1 https://github.com/PurpleI2P/i2pd.git "$I2PD_DIR"
else
    echo "Repository exists, pulling latest..."
    cd "$I2PD_DIR" && git pull && cd "$SCRIPT_DIR"
fi

# Build libi2pd static library
cd "$I2PD_DIR/build"
rm -rf obj && mkdir -p obj && cd obj

echo "Configuring for Windows (MinGW64)..."
cmake -DWITH_STATIC=ON \
      -DWITH_BINARY=OFF \
      -DWITH_UPNP=OFF \
      -DCMAKE_BUILD_TYPE=Release \
      ..

echo "Building targets..."
cmake --build . --target libi2pd libi2pdclient libi2pdlang -- -j$(nproc)

# Copy library to current directory for CGO
cp libi2pd.a libi2pdclient.a libi2pdlang.a "$SCRIPT_DIR/"
echo "Windows Build Complete ✓"
