# Homebrew Cask (macOS)

**🌐 Language**: English | [Русский](HOMEBREW.ru.md)

This repository also serves as a Homebrew tap. It supports the **universal** macOS 11+ release (Apple Silicon and Intel). The separate macOS Catalina legacy build is not included.

## Install

Because the repository is named `singbox-launcher` rather than `homebrew-singbox-launcher`, specify the Git URL when tapping:

```bash
brew tap leadaxe/singbox-launcher https://github.com/Leadaxe/singbox-launcher.git
brew install --cask leadaxe/singbox-launcher/singbox-launcher
```

On Homebrew versions with third-party tap trust checks, installing the fully qualified Cask should trust only that Cask. If prompted, review the Cask before granting trust.

Updates work through the usual Homebrew commands:

```bash
brew update
brew upgrade --cask leadaxe/singbox-launcher/singbox-launcher
```

## How this differs from the upstream install script

- The Cask installs the `.app` only. It does not download the separately distributed sing-box core and configuration template ahead of time, or set up the privileged TUN helper. The application may need internet access and administrator authorization to perform initial setup or enable TUN. For an offline-ready first launch, the project's `scripts/install-macos.sh` remains the recommended approach.
- The upstream app is not currently notarized for macOS. Gatekeeper may block opening it. This Cask deliberately **does not** remove quarantine attributes or disable Gatekeeper. Check the [macOS installation instructions](../README.md#macos) and use the macOS security UI if you decide to allow the app.
- If migrating from a pre-v2.3 installation, export an **LX Backup** before upgrading: older versions stored user data inside the application bundle, which replacement can erase.

## Maintaining the Cask

`Casks/singbox-launcher.rb` is pinned to the latest stable release with its published SHA-256. `scripts/update-homebrew-cask.sh` reads the GitHub Release metadata (falling back to `checksums.txt`) and modifies **only** the version and SHA. The [Update Homebrew Cask workflow](../.github/workflows/update-homebrew-cask.yml) runs when the existing `CI/CD` workflow finishes, once daily as a fallback, and on demand; it opens or updates a **draft PR** for review.

**Maintainer action required:** GitHub requires enabling **Settings → Actions → General → Workflow permissions → Allow GitHub Actions to create and approve pull requests** for the repository, otherwise the automatic PR step fails. The workflow requests `contents: write` and `pull-requests: write` for this purpose.

To test a proposed version manually, run `brew audit --cask ./Casks/singbox-launcher.rb` and test the installation on macOS. The upstream archive currently stores the app under `artifacts-darwin/` (the release CI artifact directory), so the `app` stanza intentionally uses that nested path.
