# Sing-Box Launcher

[![GitHub](https://img.shields.io/badge/GitHub-Leadaxe%2Fsingbox--launcher-blue)](https://github.com/Leadaxe/singbox-launcher)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24%2B-blue)](https://golang.org/)
[![Version](https://img.shields.io/badge/version-0.8.4-blue)](https://github.com/Leadaxe/singbox-launcher/releases)

Cross-platform GUI launcher for [sing-box](https://github.com/SagerNet/sing-box) - universal proxy client.

**Repository**: [https://github.com/Leadaxe/singbox-launcher](https://github.com/Leadaxe/singbox-launcher)

**🌐 Languages**: [English](README.md) | [Русский](README_RU.md)

## 📑 Table of Contents

- [📸 Screenshots](#-screenshots)
- [🚀 Features](#-features)
- [💡 Why this launcher?](#-why-this-launcher)
- [📋 Requirements](#-requirements)
- [📦 Installation](#-installation)
- [📖 Usage](#-usage)
  - [First Launch](#first-launch)
  - [Main Features](#main-features)
  - [Config Wizard (v0.2.0)](#config-wizard-v020)
  - [System Tray](#system-tray)
- [⚙️ Configuration](#️-configuration)
  - [Config Template (config_template.json)](#config-template-config_templatejson)
  - [Enabling Clash API](#enabling-clash-api)
  - [Subscription Parser Configuration](#subscription-parser-configuration)
- [🔄 Subscription Parser](#-subscription-parser)
- [🏗️ Project Architecture](#️-project-architecture)
- [🐛 Troubleshooting](#-troubleshooting)
- [🔁 Auto-restart & Stability](#-auto-restart--stability)
- [🔨 Building from Source](#-building-from-source)
- [🤝 Contributing](#-contributing)
- [📄 License](#-license)
- [🙏 Acknowledgments](#-acknowledgments)
- [📞 Support](#-support)
- [🔮 Future Plans](#-future-plans)

## 📸 Screenshots

### Core Dashboard
![Core Dashboard](https://github.com/user-attachments/assets/660d5f8d-6b2e-4dfa-ba6a-0c6906b383ee)

### Clash API
![Clash API Dashboard](https://github.com/user-attachments/assets/389e3c08-f92e-4ef1-bea1-39074b9b6eca)

![Clash API in Tray](https://github.com/user-attachments/assets/9801820b-501c-4221-ba56-96f3442445b0)

### Config Wizard
![Config Wizard](https://github.com/user-attachments/assets/07d290c1-cdab-4fd4-bd12-a39c77b3bd68)

## 🚀 Features

- ✅ **Cross-platform**: Windows (fully tested), macOS (fully tested), Linux (testing needed - help welcome!)
- 🎯 **Simple Control**: Start/stop VPN with one button
- 🧙 **Config Wizard**: Visual step-by-step configuration without editing JSON
- 📊 **Clash API Integration**: Manage proxies via Clash-compatible API
- 🤖 **Auto-loaders**: Automatic proxy loading from Clash API on startup
- 🔄 **Automatic Configuration Update**: Parse subscriptions and update proxy list
- 🔁 **Auto-restart**: Intelligent crash recovery with stability monitoring
- 📈 **Diagnostics**: IP check, STUN, file verification
- 🔔 **System Tray**: Run from system tray with proxy selection
- 📝 **Logging**: Detailed logs of all operations

## 💡 Why this launcher?

### ❌ The Problem

Most Windows users run sing-box like this:

- 📁 `sing-box.exe` + `config.json` in the same folder  
- ⚫ Black CMD window always open  
- ✏️ To switch a node: edit JSON in Notepad → kill the process → run again  
- 📝 Logs disappear into nowhere  
- 🔄 Manual restart every time you change config

### ✅ The Solution

This launcher solves all of that. Everything is controlled from one clean GUI:

### 🎯 What it gives you

- 🚀 **One-click start/stop for TUN mode**  
- 📝 **Full access to `config.json` inside the launcher**  
  (edit → save → sing-box restarts automatically)
- 🔄 **Auto-parsing of any subscription type**  
  (vless / vmess / trojan / ss / hysteria / hysteria2 / tuic)  
  + filters by tags and regex
- 🌐 **Server selection with ping via Clash Meta API**  
- 🔧 **Diagnostics tools**: IP-check, STUN test, process killer  
- 📊 **System tray integration + readable logs**

**🔗 Links:**
- **GitHub:** https://github.com/Leadaxe/singbox-launcher  
- **Example config:** https://github.com/Leadaxe/singbox-launcher/blob/main/bin/config.example.json

## 📋 Requirements

### Windows
- **Recommended systems:** Windows 10/11 (x64)
- **Compatibility mode:** Windows 7 (x86/x64) via a separate legacy build `singbox-launcher-<version>-win7-32.zip`  
  In this mode the launcher uses a fixed legacy `sing-box` version (1.13.2, 32-bit) and 32-bit `wintun.dll`, both working on Win7 x86 and Win7 x64.
- [sing-box](https://github.com/SagerNet/sing-box/releases) (executable file)
- [WinTun](https://www.wintun.net/) (wintun.dll) - MIT license, can be distributed

### macOS

**Requirements:**
- **Universal build** (recommended): macOS 11.0+ (Big Sur or newer) - supports both Apple Silicon and Intel Macs
- **Intel-only build**: macOS 10.15+ (Catalina or newer) - Intel Macs only
- [sing-box](https://github.com/SagerNet/sing-box/releases) (executable file)

### Linux

**⚠️ Note**: Linux builds are not available. The build process and functionality need testing. We're looking for help with testing and feedback!

**Requirements:**
- Modern Linux distribution (x64)
- [sing-box](https://github.com/SagerNet/sing-box/releases) (executable file)

If you can help test on Linux, please open an issue or pull request on GitHub!

## 📦 Installation

### Windows

1. Download the latest release from [GitHub Releases](https://github.com/Leadaxe/singbox-launcher/releases)  
   - for Windows 10/11 (x64) — regular Windows release archive;
   - for Windows 7 (x86/x64) — `singbox-launcher-<version>-win7-32.zip` legacy build.
2. Extract the archive to any folder (e.g., `C:\Program Files\singbox-launcher`)
3. Place `config.json` in the `bin\` folder:
   - Copy `config.example.json` to `config.json` and configure it
4. Run `singbox-launcher.exe`
5. **Automatic download** (recommended):
   - Go to the **"Core"** tab
   - Click **"Download"** to download `sing-box.exe` (the launcher will automatically choose a compatible binary for your platform; on Windows 7 it always uses the fixed 32-bit 1.13.2 legacy build)
   - Click **"Download wintun.dll"** if needed (automatically downloads the correct architecture; on Windows 7 — 32-bit `wintun.dll`)
   - The launcher will automatically download from GitHub or SourceForge mirror if GitHub is unavailable

### macOS

There are two ways to install on macOS:

#### Option 1: Installation Script (Recommended)

**⚠️ Important**: If you encounter compatibility issues (e.g., "This app cannot be used with this version of macOS" on Apple Silicon or macOS Sequoia), use the installation script instead of manual installation.

**Quick Install (Latest Version):**

1. Open Terminal (Press `Cmd + Space`, type "Terminal", press Enter)
2. Copy and paste this command:

```bash
curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash
```

3. Press Enter and follow the prompts (if asked about restoring config, press Enter to skip)

The script will automatically:
- Detect and download the latest version
- Install to `/Applications/`
- Fix macOS quarantine attributes and permissions
- Ensure compatibility with Apple Silicon and all macOS versions
- Open Finder to show the installed app

**Install Specific Version:**

```bash
curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash -s -- v0.8.0
```

Replace `v0.8.0` with the version you want to install.

#### Option 2: Manual Installation

1. Download the latest release for macOS from [GitHub Releases](https://github.com/Leadaxe/singbox-launcher/releases)
2. Extract the ZIP archive
3. Remove quarantine attribute (required on macOS):
   ```bash
   xattr -cr "singbox-launcher.app" && chmod +x "singbox-launcher.app/Contents/MacOS/singbox-launcher"
   ```
4. For .app bundle: Double-click `singbox-launcher.app` to run

   Or from command line:
   ```bash
   open singbox-launcher.app
   ```

   If macOS still blocks the app, go to **System Settings → Privacy & Security** and click **"Open Anyway"**, or right-click the app and select **"Open"** (first time only).

### Linux

**⚠️ Note**: Linux builds are not available. You need to build from source. The build process and functionality need testing. If you encounter issues, please report them on GitHub.

**To build and run:**
1. Build from source (see [Building from Source](#-building-from-source) section)
2. Make executable and run:
   ```bash
   chmod +x singbox-launcher
   ./singbox-launcher
   ```
   
   The launcher will automatically download `sing-box` and other required files on first launch.

**We're looking for help**: If you can test on Linux and provide feedback, please open an issue on [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)!

## 📖 Usage

### First Launch

#### Option 1: Using Config Wizard (Recommended)

1. **Download sing-box and wintun.dll** (if not already present):
   - Open the **"Core"** tab
   - Click **"Download"** to download `sing-box` (automatically detects your platform)
   - On Windows, click **"Download wintun.dll"** if needed
   - Files will be downloaded to the `bin/` folder automatically

2. **Configure using Wizard**:
   - If `config.json` is missing, click the blue **"Wizard"** button in the **"Core"** tab
   - If `config_template.json` is missing, click **"Download Config Template"** first
   - Follow the wizard steps:
     - **Tab 1 (Sources & ParserConfig)**: Enter subscription URL, configure ParserConfig
     - **Tab 2 (Rules)**: Select routing rules, configure outbound selectors
     - **Tab 3 (Preview)**: Review generated configuration and save
   - The wizard will create `config.json` automatically

3. Click the **"Start"** button in the **"Core"** tab to start sing-box

#### Option 2: Manual Configuration

1. Configure `config.json` manually (see [Configuration](#-configuration) section)
2. **Download sing-box and wintun.dll** (if not already present):
   - Open the **"Core"** tab
   - Click **"Download"** to download `sing-box` (automatically detects your platform)
   - On Windows, click **"Download wintun.dll"** if needed
3. Click the **"Start"** button in the **"Core"** tab to start sing-box

### Main Features

#### "Core" Tab

![Core Dashboard](https://github.com/user-attachments/assets/660d5f8d-6b2e-4dfa-ba6a-0c6906b383ee)

- **Core Status** - Shows sing-box running status (Running/Stopped/Error)
  - Displays restart counter during auto-restart attempts (e.g., `[restart 2/3]`)
  - Counter automatically resets after 3 minutes of stable operation
- **Sing-box Ver.** - Displays installed version (clickable on Windows to open file location)
- **Update** button (🔄) - Download or update sing-box binary
- **WinTun DLL** (Windows only) - Shows wintun.dll status and download button
- **Config Status** - Shows config.json status and last modification date (YYYY-MM-DD)
- **Wizard** button (⚙️) - Open configuration wizard (blue if config.json is missing)
- **Update Config** button (🔄) - Update configuration from subscriptions (disabled if config.json is missing)
- **Download Config Template** button - Download config_template.json (blue if template is missing)
- Automatic fallback to SourceForge mirror if GitHub is unavailable

#### "Diagnostics" Tab
- **Check Files** - Check for required files
- **Check STUN** - Determine external IP via STUN
- Buttons to check IP on various services

#### "Tools" Tab
- **Open Logs Folder** - Open logs folder
- **Open Config Folder** - Open configuration folder
- **Kill Sing-Box** - Force kill sing-box process

#### "Clash API" Tab

![Clash API Dashboard](https://github.com/user-attachments/assets/389e3c08-f92e-4ef1-bea1-39074b9b6eca)

- **Test API Connection** - Test Clash API connection
- **Load Proxies** - Load proxy list from selected group
- Switch between proxy servers
- Check latency (ping) for each proxy
- **Auto-loaders**: Automatically loads proxies when sing-box starts
- Tab is visually disabled (grayed out) when sing-box is not running

### Config Wizard (v0.2.0)

The Config Wizard provides a visual interface for configuring sing-box without manually editing JSON files.

![Config Wizard](https://github.com/user-attachments/assets/07d290c1-cdab-4fd4-bd12-a39c77b3bd68)

**Accessing the Wizard:**
- Click the **"Wizard"** button (⚙️) in the **"Core"** tab
- The button is blue (high importance) if `config.json` is missing

**Wizard Tabs:**

1. **Sources & ParserConfig**
   - Enter subscription URL or direct links (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://) and validate connectivity
   - Supports both subscription URLs and direct links (can be combined, separated by line breaks)
   - Configure ParserConfig JSON with visual editor
   - Preview generated outbounds
   - Parse subscription and generate proxy list

2. **Rules**

![Clash API in Tray](https://github.com/user-attachments/assets/9801820b-501c-4221-ba56-96f3442445b0)

   - **Template Rules**: Select routing rules from template
     - Rules marked with `@default` directive are enabled by default
     - Configure outbound selectors for each rule
     - Rules with `?` button have descriptions (hover or click to view)
   
   - **Custom Rules**: Create your own routing rules
     - Click **"➕ Add Rule"** button to create a new rule
     - Choose rule type: **IP Addresses (CIDR)** or **Domains/URLs**
     - Enter rule name and IP addresses/domains (one per line)
     - Select outbound for the rule
     - Click **"Add"** to save the rule
     - Click **"✏️"** (edit) button to modify an existing custom rule
     - Click **"❌"** (delete) button to remove a custom rule
     - Custom rules appear in the same list as template rules
   
   - **Final Outbound**: Select default outbound for unmatched traffic
   - **Preview Auto-refresh**: Preview automatically regenerates when you switch to Preview tab after making changes
   - Scrollable list (70% of window height)

3. **Preview**
   - Real-time preview of generated configuration
   - JSON validation before saving (supports JSONC with comments)
   - Automatic backup of existing config (`config-old.json`, `config-old-1.json`, etc.)
   - Auto-closes after successful save

**Features:**
- Loads existing configuration if available
- Uses `config_template.json` for default rules
- Supports custom user-defined rules (IP addresses or domains/URLs)
- Automatic preview regeneration when switching tabs after rule changes
- Supports JSONC (JSON with comments)
- Automatic backup before saving
- Navigation: Close/Next buttons on first two tabs, Close/Save on last tab

### System Tray

The application runs in the system tray. Click the icon to:
- Open the main window
- Start/stop VPN
- Select proxy server (if Clash API is enabled)
- Exit the application

**Auto-loaders**: Proxies are automatically loaded from Clash API when sing-box starts.

## ⚙️ Configuration

### Folder Structure

```
singbox-launcher/
├── bin/
│   ├── sing-box.exe (or sing-box for Unix) - auto-downloaded via Core tab
│   ├── wintun.dll (Windows only) - auto-downloaded via Core tab
│   ├── config.json - main configuration (created via wizard or manually)
│   └── config_template.json - template for wizard (auto-downloaded if missing)
├── logs/
│   ├── singbox-launcher.log
│   ├── sing-box.log
│   └── api.log
└── singbox-launcher.exe (or singbox-launcher for Unix)
```

**Note:** `sing-box`, `wintun.dll`, and `config_template.json` can be downloaded automatically through the **Core** tab. The launcher will:
- Automatically detect your platform (Windows/macOS/Linux) and architecture (amd64/arm64)
- Download the correct version from GitHub or SourceForge mirror (if GitHub is blocked)
- Install files to the correct location

**Platform Support**: Windows and macOS are fully supported.

### Configuring config.json

The launcher uses the standard sing-box configuration file. Detailed documentation is available on the [official sing-box website](https://sing-box.sagernet.org/configuration/).

#### Using Config Wizard

The easiest way to configure is using the **Config Wizard**:
1. Click **"Wizard"** button (⚙️) in the **"Core"** tab
2. Follow the step-by-step instructions
3. The wizard will generate a valid `config.json` automatically

#### Manual Configuration

If you prefer to edit `config.json` manually, see the sections below.

#### Config Template (config_template.json)

The `config_template.json` file provides a template for the Config Wizard and defines selectable routing rules. **This single file works for all platforms** (Windows, macOS, Linux).

**Template Structure:**

The unified template uses a clean JSON structure with four main sections:

1. **`parser_config`** - Default parser configuration (subscriptions, outbound groups, update intervals)
2. **`config`** - Main sing-box configuration (platform-independent part: log, dns, route, etc.)
3. **`selectable_rules`** - User-selectable routing rules (appear as checkboxes in the wizard)
4. **`params`** - Platform-specific configuration overrides (applied based on `runtime.GOOS`)

**Key Features:**

- **No comment-based directives** - Pure JSON structure, easy to validate and maintain
- **Platform-specific via `params`** - Single template file with platform-specific sections applied automatically
- **Self-contained rules** - Each `selectable_rule` includes its own `rule_set` definitions (only loaded when rule is enabled)
- **Platform filtering** - Rules can be filtered by platform using the `platforms` field

**Outbound Selection:**

When a rule has an `outbound` field, the wizard provides a dropdown with the following options:

1. **Generated outbounds** - All outbounds created from subscriptions (e.g., `proxy-out`, `🇳🇱Netherlands`, etc.)
2. **`direct-out`** - Always available for direct connections (bypass proxy)
3. **`reject`** - Always available for blocking traffic (converted to `"action": "reject", "method": "drop"` in config)

**Example Template Structure:**

```json
{
  "parser_config": {
    "ParserConfig": {
      "version": 4,
      "proxies": [{ "source": "https://your-subscription-url-here" }],
      "outbounds": [ /* proxy groups */ ]
    }
  },
  "config": {
    "log": { /* logging config */ },
    "dns": { /* DNS config */ },
    "inbounds": [],
    "outbounds": [{ "type": "direct", "tag": "direct-out" }],
    "route": { /* routing config */ }
  },
  "selectable_rules": [
    {
      "label": "Block Ads",
      "description": "Soft-block ads by rejecting connections",
      "default": true,
      "rule_set": [ /* rule set definitions */ ],
      "rule": { "rule_set": "ads-all", "action": "reject" }
    }
  ],
  "params": [
    {
      "name": "inbounds",
      "platforms": ["windows", "linux"],
      "value": [ /* TUN configuration */ ]
    }
  ]
}
```

**Creating Custom Templates:**

You can create your own `config_template.json` file to customize the rules available in the Config Wizard:

1. **Start with the default template**: Download the default template using the **"Download Config Template"** button
2. **Edit the template**: Modify `config_template.json` in the `bin/` folder
3. **Add custom rules**: Add entries to the `selectable_rules` array with `label`, `description`, `rule`, and optional `rule_set`
4. **Customize ParserConfig**: Modify the `parser_config` section to set default subscription settings
5. **Add platform-specific settings**: Use the `params` section to add platform-specific configurations
6. **Save and use**: The wizard will automatically use your custom template

**User-Defined Custom Rules:**

In addition to template rules, users can create their own rules directly in the wizard:

- **IP Address Rules**: Specify IP addresses or CIDR ranges (e.g., `192.168.1.0/24`, `10.0.0.1`)
- **Domain/URL Rules**: Specify domains or URLs (e.g., `example.com`, `*.example.com`)
- Custom rules are saved in `config.json` and persist between wizard sessions
- Custom rules appear alongside template rules in the Rules tab
- Each custom rule can have its own outbound selector
- Custom rules support the same outbound options as template rules (generated outbounds, `direct-out`, `reject`)

**Rule Format in config.json:**

Custom rules are saved in the standard sing-box rule format:

```json
{
  "route": {
    "rules": [
      // Template rules...
      {
        "ip_cidr": ["192.168.1.0/24", "10.0.0.1"],
        "outbound": "proxy-out"
      },
      {
        "domain": ["example.com", "*.example.com"],
        "outbound": "direct-out"
      }
    ]
  }
}
```

**📖 Complete Guide for VPN Providers:**

For detailed instructions on creating your own `config_template.json` template, see:
- **[docs/CREATE_WIZARD_TEMPLATE.md](docs/CREATE_WIZARD_TEMPLATE.md)** - Complete guide with examples and best practices
- The guide covers the unified JSON structure, platform-specific configurations, DNS setup, TUN vs System Proxy, and local traffic rules

**Note:** The template file must be valid JSON. The wizard validates the template before use.

#### Enabling Clash API

To use the "Clash API" tab, add to `config.json`:

```json
{
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:9090",
      "secret": "your-secret-token-here"
    }
  }
}
```

#### Subscription Parser Configuration

For automatic configuration updates from subscriptions, configure the `parser_config` section in `config_template.json` or use the Config Wizard.

**Using Config Wizard (Recommended):**

1. Open the Config Wizard (click **"Wizard"** button in the **"Core"** tab)
2. Go to **"Sources & ParserConfig"** tab
3. Enter your subscription URL or direct links
4. Configure ParserConfig JSON in the visual editor
5. The wizard will save the configuration to `config.json`

**Manual Configuration:**

If you prefer to edit manually, the parser configuration is stored in `config.json` (loaded from `config_template.json` by default). The structure follows the ParserConfig format:

```json
{
  "ParserConfig": {
    "version": 4,
    "proxies": [
      {
        "source": "https://your-subscription-url.com/subscription",
        "connections": [
          "vless://uuid@server.com:443?security=reality&...#ServerName",
          "vmess://eyJ2IjoiMiIsInBzIjoi..."
        ]
      }
    ],
    "outbounds": [
      {
        "tag": "proxy-out",
        "type": "selector",
        "options": { "interrupt_exist_connections": true },
        "filters": {
          "tag": "!/(🇷🇺)/i"
        },
        "addOutbounds": ["direct-out"],
        "preferredDefault": { "tag": "/🇳🇱/i" },
        "comment": "Proxy group for international connections"
      }
    ],
    "parser": {
      "reload": "4h"
    }
  }
}
```

**📖 For detailed parser configuration documentation, see [docs/ParserConfig.md](docs/ParserConfig.md)**

**Note:** You can configure all of this visually via the Config Wizard (recommended for beginners). Manual JSON editing is for advanced users.

## 🔄 Subscription Parser

The subscription parser automatically updates the proxy server list in `config.json` from subscriptions.

### Overview

The parser reads the `ParserConfig` section from `config.json` (or `config_template.json`), downloads subscriptions, filters nodes, and generates selectors according to your configuration.

**Key Features:**
- Supports multiple subscription URLs and direct links (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://)
- Flexible filtering by tags, protocols, and other parameters
- Automatic grouping into selectors
- Automatic configuration reload based on time intervals
- Automatic migration from older configuration versions

**📖 For detailed parser configuration documentation, see [docs/ParserConfig.md](docs/ParserConfig.md)**

## 🏗️ Project Architecture

```
singbox-launcher/
├── api/              # Clash API client
├── assets/           # Icons and resources
├── bin/              # Executables and configuration
├── build/            # Build scripts
├── core/             # Core application logic
├── internal/         # Internal packages
│   └── platform/     # Platform-specific code
│       ├── platform_windows.go
│       ├── platform_darwin.go
│       └── platform_common.go
├── ui/               # User interface
├── logs/             # Application logs
├── main.go           # Entry point
├── go.mod            # Go dependencies
└── README.md         # This file
```

### Cross-platform

The project uses build tags for conditional compilation of platform-specific code:

- `//go:build windows` - code for Windows
- `//go:build darwin` - code for macOS
- `//go:build linux` - code for Linux

Platform-specific functions are in the `internal/platform` package.

## 🐛 Troubleshooting

### Sing-box won't start

1. **Download sing-box** if missing:
   - Go to the **"Core"** tab
   - Click **"Download"** to download sing-box automatically
   - On Windows, also download `wintun.dll` if TUN mode is used
2. **Use Config Wizard** to create valid configuration:
   - Click **"Wizard"** button (⚙️) in the **"Core"** tab
   - Follow the wizard steps
3. Check that `sing-box.exe` (or `sing-box`) file exists in the `bin/` folder
4. Check `config.json` correctness
5. Check logs in the `logs/` folder

### Config Wizard not working

1. **Download config template** if missing:
   - Click **"Download Config Template"** button in the **"Core"** tab
2. Make sure `config_template.json` exists in the `bin/` folder
3. Check that the template file is valid JSON

### Clash API not working

1. Make sure `experimental.clash_api` is enabled in `config.json`
2. Check that sing-box is running (tab is disabled when not running)
3. Check logs in `logs/api.log`

### Permission issues (Linux/macOS)

**Note**: Linux builds are not available. If you build from source and encounter issues, please report them.

On Linux/macOS, administrator rights may be required to create TUN interface:

```bash
sudo ./singbox-launcher
```

Or configure permissions via `setcap` (Linux):

```bash
sudo setcap cap_net_admin+ep ./singbox-launcher
```

## 🔁 Auto-restart & Stability

The launcher includes intelligent auto-restart functionality:

**Features:**
- Automatic restart on crashes (up to 3 attempts)
- 2-second delay before restart to allow proper cleanup
- Stability monitoring: counter resets after 180 seconds (3 minutes) of stable operation
- Visual feedback: restart counter displayed in Core Status (e.g., `[restart 2/3]`)
- No false warnings during auto-restart attempts
- Status automatically updates when counter resets

**Behavior:**
- If sing-box crashes, the launcher will automatically attempt to restart it
- After 3 failed attempts, it stops and shows an error message
- If sing-box runs stably for 3 minutes after a restart, the counter resets
- Status automatically updates when counter resets

## 🔨 Building from Source

### Prerequisites

- Go 1.24 or newer
- Git
- For Windows: [rsrc](https://github.com/akavel/rsrc) for embedding icons (optional)

### Windows

**Requirements:**
- Go 1.24 or newer ([download](https://go.dev/dl/))
- **C Compiler (GCC)** - REQUIRED! ([TDM-GCC](https://jmeubank.github.io/tdm-gcc/) or [MinGW-w64](https://www.mingw-w64.org/))
- CGO (enabled by default)
- Optional: `rsrc` for embedding icon (`go install github.com/akavel/rsrc@latest`)

**⚠️ Important:** If you see error `gcc: executable file not found`, install GCC (see [docs/BUILD_WINDOWS.md](docs/BUILD_WINDOWS.md) "Troubleshooting" section)

**Build:**

1. Clone the repository:
```batch
git clone https://github.com/Leadaxe/singbox-launcher.git
cd singbox-launcher
```

2. Run the build script:
```batch
build\build_windows.bat
```

Or manually:
```batch
go mod tidy
go build -buildvcs=false -ldflags="-H windowsgui -s -w" -o singbox-launcher.exe
```

**Detailed instructions:** See [docs/BUILD_WINDOWS.md](docs/BUILD_WINDOWS.md)

### macOS

**Requirements:**
- Full Xcode (not just Command Line Tools) - required for universal binary builds
- Go 1.24 or newer

**Build options:**

1. **Universal binary** (recommended - default):
   - Supports both Apple Silicon (arm64) and Intel (x86_64) Macs
   - Requires macOS 11.0+ (Big Sur or newer)
   - Creates `.app` bundle with proper Info.plist configuration

```bash
# Clone the repository
git clone https://github.com/Leadaxe/singbox-launcher.git
cd singbox-launcher

# Build universal binary (default)
chmod +x build/build_darwin.sh
./build/build_darwin.sh
# or explicitly:
./build/build_darwin.sh universal
```

2. **Intel-only binary** (for older Macs):
   - Supports Intel Macs only
   - Requires macOS 10.15+ (Catalina or newer)
   - Useful if you need to support older Intel Macs

```bash
# Build Intel-only binary
./build/build_darwin.sh intel
```

**Build script features:**
- Automatically creates universal binary (arm64 + x86_64) or Intel-only binary
- Creates proper `.app` bundle structure with Info.plist
- Sets correct `LSMinimumSystemVersion` and architecture priorities
- Includes application icon if available

**Manual build** (not recommended - won't create .app bundle):
```bash
GOOS=darwin GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o singbox-launcher
```

### Linux

**⚠️ Note**: Linux builds are not available. The build process and functionality need testing. Help is welcome!

**Linux:**
```bash
# Clone the repository
git clone https://github.com/Leadaxe/singbox-launcher.git
cd singbox-launcher

# Install dependencies
go mod download

# Build the project
chmod +x build/build_linux.sh
./build/build_linux.sh
```

Or manually:
```bash
GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o singbox-launcher
```

**Help Wanted**: If you can test builds on Linux, please share your feedback on [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)!

## 🤝 Contributing

## 🧪 Running tests

- **Preferred (recommended):** use the centralized test scripts in `build/` which explicitly filter GUI packages that require OpenGL/`fyne`.

- Linux (runner / local):
  ```bash
  ./build/test_linux.sh
  ```

- macOS:
  ```bash
  ./build/test_darwin.sh
  ```

- Windows:
  ```bat
  build\test_windows.bat
  ```

- These scripts exclude UI packages (`/ui/`) and packages importing `fyne.io` to avoid CI failures on headless runners. If you need to run GUI/integration tests locally, run `build/test_windows.bat run <TestName>` or set `TEST_PACKAGE` manually in the script.

Note: root-level `test.sh`/`test.bat` were replaced with lightweight wrappers delegating to `build/`.

We welcome contributions to the project! Please:

1. Fork the repository
2. Create a branch for your feature (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

### Code Style

- Follow Go standards: `gofmt`, `golint`
- Add comments to public functions
- Write tests for new functionality

## 📄 License

This project is distributed under the MIT license. See the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [SagerNet/sing-box](https://github.com/SagerNet/sing-box) - for excellent proxy client
- [Fyne](https://fyne.io/) - for cross-platform UI framework
- All project contributors

## 📞 Support

- **Telegram**: [@singbox_launcher](https://t.me/singbox_launcher) - Discussion channel
- **Issues**: [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)


---

**Note**: This project is not affiliated with the official sing-box project. This is an independent development for convenient sing-box management.
