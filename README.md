# Sing-Box Launcher

[![GitHub](https://img.shields.io/badge/GitHub-Leadaxe%2Fsingbox--launcher-blue)](https://github.com/Leadaxe/singbox-launcher)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24%2B-blue)](https://golang.org/)
[![Version](https://img.shields.io/badge/version-0.2.0-blue)](https://github.com/Leadaxe/singbox-launcher/releases)

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
- [🔄 Subscription Parser - Detailed Logic](#-subscription-parser---detailed-logic)
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

- ✅ **Cross-platform**: Windows (fully tested), macOS and Linux (testing needed - help welcome!)
- 🎯 **Simple Control**: Start/stop VPN with one button
- 🧙 **Config Wizard** (v0.2.0): Visual step-by-step configuration without editing JSON
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
  (vless / vmess / trojan / ss / hysteria / tuic)  
  + filters by tags and regex
- 🌐 **Server selection with ping via Clash Meta API**  
- 🔧 **Diagnostics tools**: IP-check, STUN test, process killer  
- 📊 **System tray integration + readable logs**

**🔗 Links:**
- **GitHub:** https://github.com/Leadaxe/singbox-launcher  
- **Example config:** https://github.com/Leadaxe/singbox-launcher/blob/main/bin/config.example.json

## 📋 Requirements

### Windows
- Windows 10/11 (x64)
- [sing-box](https://github.com/SagerNet/sing-box/releases) (executable file)
- [WinTun](https://www.wintun.net/) (wintun.dll) - MIT license, can be distributed

### macOS & Linux

**⚠️ Testing Needed**: macOS and Linux support is implemented but not yet fully tested. We're looking for help with testing and feedback!

**Requirements:**
- macOS 10.15+ (Catalina or newer) or modern Linux distribution (x64)
- [sing-box](https://github.com/SagerNet/sing-box/releases) (executable file)

If you can help test on macOS or Linux, please open an issue or pull request on GitHub!

## 📦 Installation

### Windows

1. Download the latest release from [GitHub Releases](https://github.com/Leadaxe/singbox-launcher/releases)
2. Extract the archive to any folder (e.g., `C:\Program Files\singbox-launcher`)
3. Place `config.json` in the `bin\` folder:
   - Copy `config.example.json` to `config.json` and configure it
4. Run `singbox-launcher.exe`
5. **Automatic download** (recommended):
   - Go to the **"Core"** tab
   - Click **"Download"** to download `sing-box.exe` (automatically downloads the correct version for your system)
   - Click **"Download wintun.dll"** if needed (automatically downloads the correct architecture)
   - The launcher will automatically download from GitHub or SourceForge mirror if GitHub is unavailable

### macOS & Linux

**⚠️ Note**: macOS and Linux builds are available but need testing. If you encounter issues, please report them on GitHub.

1. Download the latest release for your platform
2. Extract the archive
3. Place files in the `bin/` folder:
   - `sing-box` - executable file for your platform
   - `config.json` - configuration file (create from `config.example.json`)

4. Make executable and run:
   ```bash
   chmod +x singbox-launcher
   ./singbox-launcher
   ```

**We're looking for help**: If you can test on macOS or Linux and provide feedback, please open an issue on [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)!

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
     - **Tab 1 (VLESS Sources & ParserConfig)**: Enter subscription URL, configure ParserConfig
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

1. **VLESS Sources & ParserConfig**
   - Enter subscription URL and validate connectivity
   - Configure ParserConfig JSON with visual editor
   - Preview generated outbounds
   - Parse subscription and generate proxy list

2. **Rules**

![Clash API in Tray](https://github.com/user-attachments/assets/9801820b-501c-4221-ba56-96f3442445b0)

   - Select routing rules from template
   - Configure outbound selectors for each rule
   - Rules marked with `@default` directive are enabled by default
   - Select final outbound for default route
   - Scrollable list (70% of window height)

3. **Preview**
   - Real-time preview of generated configuration
   - JSON validation before saving (supports JSONC with comments)
   - Automatic backup of existing config (`config-old.json`, `config-old-1.json`, etc.)
   - Auto-closes after successful save

**Features:**
- Loads existing configuration if available
- Uses `config_template.json` for default rules
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

The `config_template.json` file provides a template for the Config Wizard and defines selectable routing rules.

**Template Directives:**

- `/** @ParcerConfig ... */` - Default parser configuration block
- `/** @SelectableRule ... */` - Defines a selectable routing rule
  - `@label` - Display name for the rule (shown in wizard)
  - `@description` - Description shown in info tooltip (optional)
  - `@default` - Rule is enabled by default when wizard opens (optional)
- `/** @PARSER_OUTBOUNDS_BLOCK */` - Marker where generated outbounds are inserted

**@SelectableRule Syntax:**

The rule body is a JSON object that defines the routing rule. If the rule contains an `outbound` field, the wizard will show a dropdown selector for that rule.

**Outbound Selection:**

When a rule has an `outbound` field, the wizard provides a dropdown with the following options:

1. **Generated outbounds** - All outbounds created from subscriptions (e.g., `proxy-out`, `🇳🇱Netherlands`, etc.)
2. **`direct-out`** - Always available for direct connections (bypass proxy)
3. **`reject`** - Always available for blocking traffic (converted to `"action": "reject", "method": "drop"` in config)

**Example Rules:**

```json
/** @SelectableRule
    @label Gemini via Gemini VPN
    @default
    @description Use dedicated Gemini VPN selector for Gemini rule set.
    { "rule_set": "gemini", "network": ["tcp", "udp"], "outbound": "proxy-out" },
*/
```

This rule has an `outbound` field, so the wizard will show a dropdown allowing you to select from available outbounds, `direct-out`, or `reject`.

```json
/** @SelectableRule
    @label Games direct
    @default
    @description Send gaming rule set traffic directly for lower latency.
    { "rule_set": "games", "network": ["tcp", "udp"], "outbound": "direct-out" },
*/
```

This rule uses `direct-out` by default, but you can change it in the wizard to any other available outbound.

```json
/** @SelectableRule
    @label Block ads
    @description Block advertising domains.
    { "rule_set": "ads", "outbound": "direct-out" },
*/
```

This rule has an `outbound` field, so the wizard will show a dropdown. If you select `reject` from the dropdown in the wizard, it will be converted in the generated config to:
```json
{ "rule_set": "ads", "action": "reject", "method": "drop" }
```

**Important:** In the template, you cannot write `"outbound": "reject"` directly. The template must use a valid outbound tag (e.g., `"direct-out"` or any generated outbound). The `reject` option appears in the wizard's dropdown, and when selected, it automatically replaces the `outbound` field with `"action": "reject", "method": "drop"` in the final configuration.

**Note:** Rules without an `outbound` field cannot be configured in the wizard - they are included as-is in the final configuration.

If the template is missing, you can download it via the **"Download Config Template"** button in the **"Core"** tab.

**Creating Custom Templates:**

You can create your own `config_template.json` file to customize the rules available in the Config Wizard:

1. **Start with the default template**: Download the default template using the **"Download Config Template"** button
2. **Edit the template**: Modify `config_template.json` in the `bin/` folder
3. **Add custom rules**: Use the `/** @SelectableRule ... */` syntax to add your own routing rules
4. **Customize ParserConfig**: Modify the `/** @ParcerConfig ... */` block to set default subscription settings
5. **Save and use**: The wizard will automatically use your custom template

**Template Structure:**
- Base configuration sections (log, dns, inbounds, route, etc.) are always included
- Rules marked with `/** @SelectableRule ... */` appear in the wizard's Rules tab
- The `/** @PARSER_OUTBOUNDS_BLOCK */` marker indicates where generated outbounds will be inserted
- Rules with `@default` directive are enabled by default in the wizard

**Note:** The template file must be valid JSONC (JSON with comments). The wizard validates the template before use.

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

For automatic configuration updates from subscriptions, add at the beginning of `config.json`:

```json
{
  /** @ParcerConfig
  {
    "ParserConfig": {
      "version": 2,
      "proxies": [
        {
          "source": "https://your-subscription-url.com/subscription"
        }
      ],
      "outbounds": [
        {
          "tag": "proxy-out",
          "type": "selector",
          "options": { "interrupt_exist_connections": true },
          "outbounds": {
            "proxies": { "tag": "!/(🇷🇺)/i" },
            "addOutbounds": ["direct-out"],
            "preferredDefault": { "tag": "/🇳🇱/i" }
          },
          "comment": "Proxy group for international connections"
        }
      ],
      "parser": {
        "reload": "4h"
      }
    }
  }
  */
  // ... rest of configuration
}
```

**📖 For detailed parser configuration documentation, see [ParserConfig.md](ParserConfig.md)**

**Note:** You can configure all of this visually via the Config Wizard (recommended for beginners). Manual JSON editing is for advanced users.

## 🔄 Subscription Parser - Detailed Logic

The subscription parser is a built-in feature that automatically updates the proxy server list in `config.json` from subscriptions (subscription URLs).

### How It Works

#### 1. Parser Configuration

At the beginning of the `config.json` file, there should be a `/** @ParcerConfig ... */` block with JSON configuration.

**Version 2 (Current)**: The `version` field is now inside `ParserConfig`. Automatic configuration reload is supported via the `parser` object.

**Backward Compatibility**: Version 1 format (with `version` at top level) is automatically migrated to version 2.

```json
{
  /** @ParcerConfig
  {
    "ParserConfig": {
      "version": 2,
      "proxies": [
        {
          "source": "https://your-subscription-url.com/subscription",
          "skip": [ { "tag": "!/🇷🇺/i" } ]
        }
      ],
      "outbounds": [
        {
          "tag": "proxy-out",
          "type": "selector",
          "options": { "interrupt_exist_connections": true },
          "outbounds": {
            "proxies": { "tag": "!/(🇷🇺)/i" },
            "addOutbounds": ["direct-out"],
            "preferredDefault": { "tag": "/🇳🇱/i" }
          },
          "comment": "Proxy group for international connections"
        }
      ],
      "parser": {
        "reload": "4h"
      }
    }
  }
  */
}
```

#### 2. Update Process

When you click the **"Update Config"** button in the "Core" tab (or use the Config Wizard):

1. **Reading Configuration**
   - Parser finds the `@ParcerConfig` block in `config.json`
   - Extracts subscription URLs from the `proxies[].source` field

2. **Loading Subscriptions**
   - For each URL from `proxies[].source`:
     - Downloads subscription content (Base64 and plain text supported)
     - Decodes and parses the proxy server list

3. **Supported Protocols**
   - ✅ VLESS
   - ✅ VMess
   - ✅ Trojan
   - ✅ Shadowsocks (SS)

4. **Information Extraction**
   - From each URI extracts:
     - **Tag**: left part of comment before `|` (e.g., `🇳🇱Netherlands`)
     - **Comment**: entire text after `#` in URI
     - **Connection parameters**: server, port, UUID, TLS settings, etc.

5. **Node Filtering**

   **`skip` filter** (at subscription level):
   - If a node matches any filter from `skip` - it is skipped
   - Example: `"skip": [ { "tag": "!/🇷🇺/i" } ]` - skip all non-Russian proxies
   
   **`proxies` filter** (at selector level):
   - Determines which nodes will be included in a specific selector
   - Example: `"proxies": { "tag": "!/(🇷🇺)/i" }` - all except Russian

   **Supported filter fields:**
   - `tag` - tag name (case-sensitive, with emoji)
   - `host` - server hostname
   - `label` - original string after `#` in URI
   - `scheme` - protocol (`vless`, `vmess`, `trojan`, `ss`)
   - `fragment` - URI fragment (equals `label`)
   - `comment` - right part of `label` after `|`

   **Pattern formats:**
   - `"literal"` - exact match (case-sensitive)
   - `"!literal"` - negation (does NOT match)
   - `"/regex/i"` - regular expression with `i` flag (case-insensitive)
   - `"!/regex/i"` - negated regular expression

6. **Grouping into Selectors**

   For each object in `outbounds[]`, a selector is created:
   
   - **`tag`**: selector name (used in UI and routing)
   - **`type`**: always `"selector"` for selectors
   - **`outbounds.proxies`**: filter for node selection (OR between objects, AND inside object)
   - **`outbounds.addOutbounds`**: additional tags added to the beginning of the list (e.g., `["direct-out"]`)
   - **`outbounds.preferredDefault`**: filter to determine default proxy
   - **`options`**: additional fields (e.g., `interrupt_exist_connections: true`)
   - **`comment`**: comment displayed before JSON selector

7. **Writing Result**

   Parser finds in `config.json` the block between markers:
   ```
   /** @ParserSTART */
   ... proxies and selectors will be here ...
   /** @ParserEND */
   ```
   
   And replaces it with:
   - List of all filtered proxy servers in JSON format
   - Selectors with proxy grouping according to specified rules
   - Comments from original URIs

### Important Notes

1. **Stop sing-box before updating**
   - Clash API may react to intermediate file
   - Use "Stop VPN" button before "Update Config"

2. **Markers are required**
   - `/** @ParserSTART */` and `/** @ParserEND */` must be in `config.json`
   - Without them, parser doesn't know where to insert the result

3. **Automatic normalization**
   - Incorrect flag `🇪🇳` is automatically replaced with `🇬🇧`
   - Normalization logic can be extended in parser code

4. **UI Integration**
   - "Clash API" tab automatically picks up selector list
   - By default, selector from `route.final` is selected (if matches)
   - Can be switched via dropdown list

5. **Multiple Subscriptions**
   - Multiple subscriptions can be specified in `proxies[]` array
   - All nodes will be merged and filtered together

### Automatic Configuration Reload (Version 2)

Starting from version 2, you can configure automatic periodic updates of your configuration:

```json
"parser": {
  "reload": "4h",
  "last_updated": "2024-01-15T14:30:00Z"
}
```

- **`reload`** (optional): Interval for automatic updates (e.g., `"4h"`, `"30m"`, `"1h30m"`). If not specified, automatic updates are disabled.
- **`last_updated`** (optional): Timestamp of the last successful update (RFC3339 format, UTC). Automatically updated by the launcher.

The launcher checks every minute if an update is needed based on the `reload` interval. If enough time has passed since `last_updated`, it automatically triggers a configuration update.

**Example:**
```json
"parser": {
  "reload": "4h"  // Update every 4 hours
}
```

**📖 For detailed parser configuration, see [ParserConfig.md](ParserConfig.md)**

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

**Note**: macOS and Linux support needs testing. If you encounter issues, please report them.

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

**⚠️ Important:** If you see error `gcc: executable file not found`, install GCC (see [BUILD_WINDOWS.md](BUILD_WINDOWS.md) "Troubleshooting" section)

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

**Detailed instructions:** See [BUILD_WINDOWS.md](BUILD_WINDOWS.md)

### macOS & Linux

**⚠️ Testing Needed**: macOS and Linux builds need testing. Help is welcome!

**macOS:**
```bash
# Clone the repository
git clone https://github.com/Leadaxe/singbox-launcher.git
cd singbox-launcher

# Install dependencies
go mod download

# Build the project
chmod +x build/build_darwin.sh
./build/build_darwin.sh
```

Or manually:
```bash
GOOS=darwin GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o singbox-launcher
```

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

**Help Wanted**: If you can test builds on macOS or Linux, please share your feedback on [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)!

## 🤝 Contributing

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

- **Issues**: [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Leadaxe/singbox-launcher/discussions)

## 🔮 Future Plans

- [ ] Automatic application updates
- [ ] Dark theme
- [ ] Multi-language support
- [ ] Traffic statistics graphs
- [ ] Integration with other VPN protocols

---

**Note**: This project is not affiliated with the official sing-box project. This is an independent development for convenient sing-box management.
