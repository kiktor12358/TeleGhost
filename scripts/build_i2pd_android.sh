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
    echo "--- OpenSSL Discovery for $ARCH ---"
    # Find crypto lib for this arch
    OPENSSL_CRYPTO_LIB=$(find "$OPENSSL_ROOT" -name "libcrypto.a" | grep "/$ARCH/" | head -n 1)
    OPENSSL_SSL_LIB=$(find "$OPENSSL_ROOT" -name "libssl.a" | grep "/$ARCH/" | head -n 1)
    # Find include (look for ssl.h)
    OPENSSL_INCLUDE_PATH=$(find "$OPENSSL_ROOT" -name "ssl.h" | grep "/$ARCH/" | head -n 1 | sed 's|/openssl/ssl.h||')
    
    if [ -z "$OPENSSL_INCLUDE_PATH" ]; then
        # Fallback to first found include if not arch-specific
        OPENSSL_INCLUDE_PATH=$(find "$OPENSSL_ROOT" -name "ssl.h" | head -n 1 | sed 's|/openssl/ssl.h||')
    fi
    
    if [ -z "$OPENSSL_CRYPTO_LIB" ] || [ -z "$OPENSSL_SSL_LIB" ] || [ -z "$OPENSSL_INCLUDE_PATH" ]; then
        echo "Warning: Could not fully locate OpenSSL for $ARCH, using legacy fallback"
        OPENSSL_INCLUDE_PATH="$OPENSSL_ROOT/$ARCH/include"
        OPENSSL_CRYPTO_LIB="$OPENSSL_ROOT/$ARCH/lib/libcrypto.a"
        OPENSSL_SSL_LIB="$OPENSSL_ROOT/$ARCH/lib/libssl.a"
    fi
    
    echo "   OpenSSL Include: $OPENSSL_INCLUDE_PATH"
    echo "   OpenSSL Crypto: $OPENSSL_CRYPTO_LIB"
    echo "   OpenSSL SSL: $OPENSSL_SSL_LIB"
    
    # Locate Boost for this Arch
    echo "--- Boost Discovery for $ARCH ---"
    
    # Find headers (version.hpp)
    BOOST_INCLUDE_PATH=$(find "$BOOST_ROOT" -name "version.hpp" | grep "boost/version.hpp" | head -n 1 | sed 's|/boost/version.hpp||')
    if [ -z "$BOOST_INCLUDE_PATH" ]; then
        echo "Warning: find could not locate version.hpp, using fallback paths"
        if [ -d "$BOOST_ROOT/include/boost" ]; then BOOST_INCLUDE_PATH="$BOOST_ROOT/include";
        elif [ -d "$BOOST_ROOT/boost" ]; then BOOST_INCLUDE_PATH="$BOOST_ROOT";
        else BOOST_INCLUDE_PATH="$BOOST_ROOT/include"; fi
    fi
    echo "   Boost Include: $BOOST_INCLUDE_PATH"

    # Find libs (search for filesystem lib in arch folder)
    # PurpleI2P structure is varied, so we look for libboost_filesystem.a specifically in a path containing the arch name
    BOOST_LIB_PATH=$(find "$BOOST_ROOT" -name "libboost_filesystem.a" | grep "/$ARCH/" | head -n 1 | xargs dirname)
    if [ -z "$BOOST_LIB_PATH" ]; then
        echo "Warning: find could not locate libboost_filesystem.a for $ARCH, using fallback paths"
        if [ -d "$BOOST_ROOT/lib/$ARCH" ]; then BOOST_LIB_PATH="$BOOST_ROOT/lib/$ARCH";
        elif [ -d "$BOOST_ROOT/$ARCH/lib" ]; then BOOST_LIB_PATH="$BOOST_ROOT/$ARCH/lib";
        else BOOST_LIB_PATH="$BOOST_ROOT/$ARCH/lib"; fi
    fi
    echo "   Boost Lib Dir: $BOOST_LIB_PATH"
    echo "   Boost Lib contents:"
    ls -l "$BOOST_LIB_PATH" 2>/dev/null | grep "libboost" | head -n 5 || echo "   (no libboost files found!)"

    # Export for environment detection (some CMake versions use this)
    export BOOST_ROOT="$BOOST_ROOT"
    export BOOST_INCLUDEDIR="$BOOST_INCLUDE_PATH"
    export BOOST_LIBRARYDIR="$BOOST_LIB_PATH"

    # Precise library paths for CMake as a last resort
    LIB_FS="$BOOST_LIB_PATH/libboost_filesystem.a"
    LIB_PO="$BOOST_LIB_PATH/libboost_program_options.a"

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
        -DWITH_STATIC=ON \
        -DOPENSSL_USE_STATIC_LIBS=ON \
        -DOPENSSL_ROOT_DIR="$OPENSSL_ROOT/$ARCH" \
        -DOPENSSL_INCLUDE_DIR="$OPENSSL_INCLUDE_PATH" \
        -DOPENSSL_CRYPTO_LIBRARY="$OPENSSL_CRYPTO_LIB" \
        -DOPENSSL_SSL_LIBRARY="$OPENSSL_SSL_LIB" \
        -DBOOST_ROOT="$BOOST_ROOT" \
        -DBOOST_INCLUDEDIR="$BOOST_INCLUDE_PATH" \
        -DBOOST_LIBRARYDIR="$BOOST_LIB_PATH" \
        -DBoost_INCLUDE_DIR="$BOOST_INCLUDE_PATH" \
        -DBoost_LIBRARY_DIR_RELEASE="$BOOST_LIB_PATH" \
        -DBoost_LIBRARY_DIR_DEBUG="$BOOST_LIB_PATH" \
        -DBoost_FILESYSTEM_LIBRARY_RELEASE="$LIB_FS" \
        -DBoost_PROGRAM_OPTIONS_LIBRARY_RELEASE="$LIB_PO" \
        -DBoost_FILESYSTEM_LIBRARY_DEBUG="$LIB_FS" \
        -DBoost_PROGRAM_OPTIONS_LIBRARY_DEBUG="$LIB_PO" \
        -DBoost_NO_BOOST_CMAKE=ON \
        -DBoost_NO_SYSTEM_PATHS=ON \
        -DBoost_USE_STATIC_LIBS=ON \
        -DBoost_DEBUG=ON \
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
