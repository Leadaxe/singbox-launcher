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
    This app is currently distributed without Apple notarization, so macOS
    Gatekeeper may block its first launch. Review the upstream macOS install
    instructions before deciding whether to allow it.

    This cask installs only the .app. Unlike the upstream install script,
    it does not preload the sing-box core or configure the privileged TUN
    helper. These steps may require a network connection and authorization.
  EOS
end
