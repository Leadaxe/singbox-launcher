# Homebrew Cask (macOS)

**🌐 Language**: English | [Русский](HOMEBREW.ru.md)

This repository also serves as a Homebrew tap. It supports the **universal** macOS 11+ release (Apple Silicon and Intel). The separate macOS Catalina legacy build is not included.

## Install

Because the repository is named `singbox-launcher` rather than `homebrew-singbox-launcher`, specify the Git URL when tapping:

```bash
brew tap leadaxe/singbox-launcher https://github.com/Leadaxe/singbox-launcher.git
brew install --cask leadaxe/singbox-launcher/singbox-launcher
```

Installing the fully qualified Cask trusts only this Cask, not the entire tap. The two-part name `leadaxe/singbox-launcher` is not a valid Cask reference: Homebrew uses `user/tap/cask` for third-party packages.

Once the tap is installed, you can also use the short name by trusting this particular Cask first:

```bash
brew trust --cask leadaxe/singbox-launcher/singbox-launcher
brew install --cask singbox-launcher
```

An initial installation using the full name already establishes that trust; you do **not** need to run `brew trust` a second time.

Updates work through the usual Homebrew commands:

```bash
brew update
brew upgrade --cask singbox-launcher
```

## How this differs from the upstream install script

- The Cask installs the `.app` only. It does not download the separately distributed sing-box core and configuration template ahead of time, or set up the privileged TUN helper. The application may need internet access and administrator authorization to perform initial setup or enable TUN. For an offline-ready first launch, the project's `scripts/install-macos.sh` remains the recommended approach.
- The upstream app is not currently notarized for macOS. Gatekeeper may block opening it. This Cask deliberately **does not** remove quarantine attributes or disable Gatekeeper. Check the [macOS installation instructions](../README.md#macos) and use the macOS security UI if you decide to allow the app.
- If migrating from a pre-v2.3 installation, export an **LX Backup** before upgrading: older versions stored user data inside the application bundle, which replacement can erase.

## Maintaining the Cask

`Casks/singbox-launcher.rb` pins the latest stable GitHub Release and its SHA-256. `scripts/update-homebrew-cask.sh` reads release metadata (falling back to `checksums.txt`) and changes **only** `version` and `sha256`.

The [Update Homebrew Cask workflow](../.github/workflows/update-homebrew-cask.yml) runs after a successful `CI/CD` push workflow, once per day as a fallback, or manually. It verifies Ruby syntax, downloads the published macOS ZIP, checks its SHA-256 and expected `.app` bundle, then automatically commits **only the Cask** to `main`. No manual PR is needed for future releases. A failed verification leaves the Cask unchanged. Homebrew users still need `brew update` and `brew upgrade --cask singbox-launcher` to install the update.

**Maintainer action:** The workflow requests `contents: write` for `GITHUB_TOKEN`. Repository settings and branch protection must allow the GitHub Actions bot to push this one-file change to `main`. If pushes are prohibited, the workflow fails without bypassing branch protection; change the repository policy or use the previous PR-based approach. The setting to allow Actions to create pull requests is **not** needed.

To test a proposed version manually, run `brew audit --cask ./Casks/singbox-launcher.rb` and test the installation on macOS. The upstream archive currently stores the app under `artifacts-darwin/` (the release CI artifact directory), so the `app` stanza intentionally uses that nested path.
