cask "singbox-launcher" do
  version "2.3.9"
  sha256 "92a6e29d1ae450f619e7e8a74d3f017ff5830bf4feaaea0f275329f2df675e2a"

  url "https://github.com/Leadaxe/singbox-launcher/releases/download/v#{version}/singbox-launcher-v#{version}-macos.zip"
  name "singbox-launcher"
  desc "Graphical desktop client for sing-box"
  homepage "https://github.com/Leadaxe/singbox-launcher"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on :macos

  # The upstream release ZIP keeps the app under the CI artifact directory.
  app "artifacts-darwin/singbox-launcher.app"

  caveats <<~EOS
    This app is not notarized by Apple. Gatekeeper may block its first launch.
    If you trust the download, try opening the app and then select
    System Settings > Privacy & Security > Open Anyway.

    Alternatively, you can manually remove only this app's quarantine
    attribute (default install path):
      xattr -dr com.apple.quarantine "/Applications/singbox-launcher.app"
    This bypasses Gatekeeper for the app. Do so only if you trust its source.

    This cask installs only the .app, not the sing-box core or privileged
    TUN helper. First launch/TUN setup may require downloads or authorization.
  EOS
end
