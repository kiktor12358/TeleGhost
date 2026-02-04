#!/bin/bash
# Build script for i2pd integration with TeleGhost
# This script clones and builds i2pd as a static library for CGO

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
I2PD_DIR="$SCRIPT_DIR/i2pd"

echo "=== TeleGhost i2pd Build Script ==="
echo ""

# Check dependencies
echo "[1/4] Checking dependencies..."
DEPS="g++ cmake libboost-all-dev libssl-dev zlib1g-dev"
MISSING=""

for dep in $DEPS; do
    if ! dpkg -s $dep >/dev/null 2>&1; then
        MISSING="$MISSING $dep"
    fi
done

if [ -n "$MISSING" ]; then
    echo "Missing dependencies:$MISSING"
    echo ""
    echo "Install them with:"
    echo "  sudo apt update && sudo apt install -y$MISSING"
    echo ""
    exit 1
fi

echo "All dependencies installed ✓"

# Clone i2pd if not exists
echo ""
echo "[2/4] Cloning i2pd repository..."
if [ ! -d "$I2PD_DIR" ]; then
    git clone --depth 1 https://github.com/PurpleI2P/i2pd.git "$I2PD_DIR"
    echo "Cloned ✓"
else
    echo "Already exists, pulling latest..."
    cd "$I2PD_DIR" && git pull && cd "$SCRIPT_DIR"
    echo "Updated ✓"
fi

# Build libi2pd static library
echo ""
echo "[3/4] Building libi2pd.a..."
cd "$I2PD_DIR"

# Create build directory
mkdir -p build && cd build

# Configure with CMake
cmake -DWITH_STATIC=ON \
      -DWITH_LIBRARY=ON \
      -DWITH_BINARY=OFF \
      -DWITH_UPNP=OFF \
      -DCMAKE_BUILD_TYPE=Release \
      ..

# Build
make -j$(nproc) libi2pd

echo "Built ✓"

# Build the wrapper
echo ""
echo "[4/4] Building C++ wrapper..."
cd "$SCRIPT_DIR"

g++ -std=c++17 -c i2pd_wrapper.cpp \
    -I"$I2PD_DIR/libi2pd" \
    -I"$I2PD_DIR/libi2pd_client" \
    -I"$I2PD_DIR/i18n" \
    -I"$I2PD_DIR" \
    -o i2pd_wrapper.o

# Create combined static library
ar rcs libi2pd_wrapper.a i2pd_wrapper.o

echo "Wrapper built ✓"

echo ""
echo "=== Build Complete ==="
echo ""
echo "Files created:"
echo "  - $I2PD_DIR/build/libi2pd/libi2pd.a"
echo "  - $SCRIPT_DIR/libi2pd_wrapper.a"
echo ""
echo "Now you can build TeleGhost with CGO_ENABLED=1"
