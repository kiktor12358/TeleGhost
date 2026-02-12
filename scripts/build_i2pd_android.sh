#!/bin/bash
set -e

# Arguments
ANDROID_NDK_HOME=$1
OPENSSL_ROOT=$2 # Base of OpenSSL prebuilts (containing openssl-1.1.1 or similar)
BOOST_ROOT=$3   # Base of Boost prebuilts
I2PD_SRC=$4     # Path to i2pd source

if [ -z "$ANDROID_NDK_HOME" ] || [ -z "$OPENSSL_ROOT" ] || [ -z "$BOOST_ROOT" ] || [ -z "$I2PD_SRC" ]; then
    echo "Usage: $0 <NDK_HOME> <OPENSSL_ROOT> <BOOST_ROOT> <I2PD_SRC>"
    exit 1
fi

echo "Building i2pd for Android..."
echo "NDK: $ANDROID_NDK_HOME"
echo "OpenSSL: $OPENSSL_ROOT"
echo "Boost: $BOOST_ROOT"
echo "Source: $I2PD_SRC"

# Working directory
BUILD_DIR="build_android"
mkdir -p $BUILD_DIR

# Architectures to build
ARCHS=("arm64-v8a" "armeabi-v7a" "x86_64" "x86")
# ABI names for CMake
ABIS=("arm64-v8a" "armeabi-v7a" "x86_64" "x86")

# Loop over architectures
for i in "${!ARCHS[@]}"; do
    ARCH=${ARCHS[$i]}
    ABI=${ABIS[$i]}
    
    echo ">>> Building for $ARCH ($ABI)..."
    
    ARCH_BUILD_DIR="$BUILD_DIR/$ARCH"
    mkdir -p $ARCH_BUILD_DIR
    
    # Locate OpenSSL for this Arch
    # Assuming structure: $OPENSSL_ROOT/$ARCH/lib and include
    # We used find in workflow, so OPENSSL_ROOT is likely just the parent dir?
    # No, passed OPENSSL_ROOT should be the version dir, e.g. openssl_prebuilt/openssl-1.1.1
    OPENSSL_INCLUDE="$OPENSSL_ROOT/$ARCH/include"
    OPENSSL_LIB="$OPENSSL_ROOT/$ARCH/lib"
    
    # Locate Boost for this Arch
    # Assuming structure: $BOOST_ROOT/$ARCH/include and lib
    # Similar to OpenSSL? Need to verify Boost structure.
    # If Boost is flat (headers common), libs in arch?
    BOOST_INCLUDE="$BOOST_ROOT/include" # Often common
    # Try different patterns for lib dir
    BOOST_LIB="$BOOST_ROOT/lib/$ARCH" 
    if [ ! -d "$BOOST_LIB" ]; then
         BOOST_LIB="$BOOST_ROOT/$ARCH/lib"
    fi
    if [ ! -d "$BOOST_LIB" ]; then
         echo "Error: Could not find Boost libs for $ARCH in $BOOST_ROOT"
         # Fallback to search?
    fi

    echo "   Using OpenSSL: $OPENSSL_INCLUDE"
    echo "   Using Boost Lib: $BOOST_LIB"

    # CMake Configure
    cmake -B "$ARCH_BUILD_DIR" -S "$I2PD_SRC/build" \
        -DCMAKE_TOOLCHAIN_FILE="$ANDROID_NDK_HOME/build/cmake/android.toolchain.cmake" \
        -DANDROID_ABI="$ABI" \
        -DANDROID_PLATFORM=android-21 \
        -DWITH_UPNP=NO \
        -DWITH_AESNI=NO \
        -DBUILD_SHARED_LIBS=OFF \
        -DWITH_LIBRARY=ON \
        -DWITH_BINARY=OFF \
        -DOPENSSL_ROOT_DIR="$OPENSSL_ROOT/$ARCH" \
        -DBOOST_ROOT="$BOOST_ROOT" \
        -DBOOST_INCLUDEDIR="$BOOST_INCLUDE" \
        -DBOOST_LIBRARYDIR="$BOOST_LIB" \
        -DCMAKE_BUILD_TYPE=Release

    # CMake Build
    cmake --build "$ARCH_BUILD_DIR" --config Release --jobs 4
    
    # Copy Static Libs
    DEST_DIR="internal/network/i2pd/lib/$ARCH"
    mkdir -p "$DEST_DIR"
    
    # i2pd produces libi2pd.a, libi2pdclient.a, libi2pdlang.a
    cp "$ARCH_BUILD_DIR/libi2pd.a" "$DEST_DIR/" || echo "libi2pd.a not found"
    cp "$ARCH_BUILD_DIR/libi2pdclient.a" "$DEST_DIR/" || echo "libi2pdclient.a not found"
    cp "$ARCH_BUILD_DIR/libi2pdlang.a" "$DEST_DIR/" || echo "libi2pdlang.a not found"
    
    echo ">>> Finished $ARCH"
done

echo "Build complete."
