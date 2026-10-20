cmake_minimum_required(VERSION 3.20)

# Enforcing CXX (C++) automatically checks for a working host compiler (GCC/Clang/MSVC)
project(singbox-launcher CXX)

# Verify that the Go compiler exists on the machine
find_program(GO_EXECUTABLE go)
if(NOT GO_EXECUTABLE)
    message(FATAL_ERROR "Go compiler was not found! Please install Go (https://go.dev/) and add it to your PATH.")
endif()

# 1. Platform-Specific Package Detection
if (UNIX AND NOT APPLE)
    # Linux still requires pkg-config to resolve X11/Wayland headers
    find_package(PkgConfig REQUIRED)

    set(SYSTEM_PACKAGES
        wayland-client
        wayland-cursor
        wayland-egl
        xkbcommon
        x11
        gl
        xrandr
        xcursor
        egl
        xinerama
        xi
        xxf86vm
    )

    foreach(package IN LISTS SYSTEM_PACKAGES)
        string(TOUPPER "${package}" package_upper)
        string(REPLACE "-" "_" package_upper "${package_upper}")
        pkg_check_modules(${package_upper} REQUIRED ${package})
        list(APPEND CGO_CFLAGS ${${package_upper}_CFLAGS})
        list(APPEND CGO_LDFLAGS ${${package_upper}_LDFLAGS})
    endforeach()

    if(CGO_CFLAGS)
        list(REMOVE_DUPLICATES CGO_CFLAGS)
        list(JOIN CGO_CFLAGS " " CGO_CFLAGS_STRING)
    endif()
    if(CGO_LDFLAGS)
        list(REMOVE_DUPLICATES CGO_LDFLAGS)
        list(JOIN CGO_LDFLAGS " " CGO_LDFLAGS_STRING)
    endif()

    message(STATUS "CGO_CFLAGS: ${CGO_CFLAGS_STRING}")
    message(STATUS "CGO_LDFLAGS: ${CGO_LDFLAGS_STRING}")

elseif(WIN32)
    # Windows native graphics APIs (gdi32, user32, opengl32) are resolved automatically by Go.
    message(STATUS "Configuring build for Windows. Native graphics toolchain checked.")
endif()

# 2. Determine Platform-Specific Executable Extension
if(WIN32)
    set(EXE_EXTENSION ".exe")
else()
    set(EXE_EXTENSION "")
endif()

set(OUTPUT_FILE "${CMAKE_CURRENT_BINARY_DIR}/singbox-launcher${EXE_EXTENSION}")

# 3. Dynamic Go Environment Mapping
set(GO_ENV "CGO_ENABLED=1")

if(UNIX AND NOT APPLE)
    list(APPEND GO_ENV "CGO_CFLAGS=${CGO_CFLAGS_STRING}" "CGO_LDFLAGS=${CGO_LDFLAGS_STRING}")
endif()

# 4. Add detected c++ compiler
list(APPEND GO_ENV "CXX=${CMAKE_CCXX_COMPILER}")

# Platform-specific build flags (e.g., hiding the console window on Windows GUI apps)
if(WIN32)
    set(GO_LDFLAGS "-s -w -H=windowsgui")
else()
    set(GO_LDFLAGS "-s -w")
endif()

# 5. Build Command Execution
add_custom_command(
    OUTPUT "${OUTPUT_FILE}"
    COMMAND "${CMAKE_COMMAND}" -E env
        ${GO_ENV}
        "${GO_EXECUTABLE}" build
        -buildvcs=false
        -v
        "-ldflags=${GO_LDFLAGS}"
        -o "${OUTPUT_FILE}"
    WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}"
    DEPENDS
        "${CMAKE_CURRENT_SOURCE_DIR}/go.mod"
        "${CMAKE_CURRENT_SOURCE_DIR}/go.sum"
    VERBATIM
)

add_custom_target(singbox-launcher ALL
    DEPENDS "${OUTPUT_FILE}"
)
