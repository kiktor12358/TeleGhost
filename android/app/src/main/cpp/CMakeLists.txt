cmake_minimum_required(VERSION 3.10.2)

# It's a dummy project to force Gradle to bundle libc++_shared.so
project("dummy")

add_library(
    dummy
    SHARED
    dummy.cpp
)
