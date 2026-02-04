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

if command -v pacman &> /dev/null; then
    # Arch Linux
    echo "Detected Arch Linux"
    DEPS="base-devel cmake boost openssl zlib git"
    MISSING=""
    for dep in $DEPS; do
        if ! pacman -Qi $dep &> /dev/null && ! pacman -Qg $dep &> /dev/null; then
             # base-devel is a group, so check if expanded or group exists
             if [ "$dep" == "base-devel" ]; then
                 continue # assume installed or user knows
             fi
             MISSING="$MISSING $dep"
        fi
    done
    
    if [ -n "$MISSING" ]; then
        echo "Missing dependencies:$MISSING"
        echo "Install with: sudo pacman -S$MISSING"
        exit 1
    fi

elif command -v dpkg &> /dev/null; then
    # Debian/Ubuntu
    echo "Detected Debian/Ubuntu"
    DEPS="g++ cmake libboost-all-dev libssl-dev zlib1g-dev"
    MISSING=""

    for dep in $DEPS; do
        if ! dpkg -s $dep >/dev/null 2>&1; then
            MISSING="$MISSING $dep"
        fi
    done

    if [ -n "$MISSING" ]; then
        echo "Missing dependencies:$MISSING"
        echo "Install with: sudo apt update && sudo apt install -y$MISSING"
        exit 1
    fi
else
    echo "Unknown OS. Please ensure you have: cmake, boost, openssl, zlib, g++ installed."
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
cd "$I2PD_DIR/build"

# Create binary directory
mkdir -p obj && cd obj

# Configure with CMake (CMakeLists.txt is in ../)
cmake -DWITH_STATIC=ON \
      -DWITH_LIBRARY=ON \
      -DWITH_BINARY=OFF \
      -DWITH_UPNP=OFF \
      -DCMAKE_BUILD_TYPE=Release \
      -DOPENSSL_ROOT_DIR=/usr \
      ..

# Build
make -j$(nproc) libi2pd

echo "Built ✓"

# Copy library to current directory for CGO
cp libi2pd.a "$SCRIPT_DIR/"

echo "Built libi2pd.a ✓"

echo ""
echo "=== Build Complete ==="
echo ""
echo "Now you can build TeleGhost with CGO_ENABLED=1"
