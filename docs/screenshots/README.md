# Screenshots

Screenshots referenced from the root `README.md` and `README.ru.md`.

## Required files

Save the following screenshots into this directory under these exact names:

| File | Content |
| --- | --- |
| `01-hero-core-and-wizard.png` | Two windows side-by-side: Config Wizard (Sources tab with subscription list) + main Core dashboard (Running state, version badges, Configurator button). |
| `02-server-switching.png` | Two windows side-by-side: Configurator → Outbounds tab (ParserConfig JSON + outbound list with badges) + Servers tab with selector-group dropdown open and per-server ping list. |
| `03-rules-and-hwid.png` | Two windows side-by-side: Configurator → Rules tab (preset bundles with checkboxes + Final outbound) + Settings tab showing Subscription identification section (Send device ID, Hash device model, Device ID/HWID with regenerate). |
| `04-dns-configuration.png` | Configurator → DNS tab (DNS servers list, strategy, rules) + main window About section. |
| `05-tun-settings-and-diagnostics.png` | Configurator → Settings tab (TUN, proxy-in, route strategy, TLS store) + main window Diagnostics column (Log window, Logs folder, Config folder, Kill Sing-Box, Traffic Profiler buttons, STUN, Debug API panel). |
| `06-traffic-profiler.png` | Traffic Profiler window with an Event detail dialog open — showing Time, Kind (TCPClose), Process (full path), Confidence, Domain, IP, Outbound chain, Rule, Up/Down bytes, Duration. |

## Conventions

- Format: PNG, retina-density (2x) if available.
- Window backgrounds: dark theme (matches existing app styling).
- Sensitive data: HWID regenerated, subscription URLs replaced with `https://your-subscription-url-here` if shown clearly.
- macOS chrome (traffic-light buttons) is fine — communicates cross-platform via the screenshots themselves.
