# Implementation Report — SPEC 081

**Дата:** 2026-06-17 · **Статус:** Complete · **Проверено на:** macOS 11.7.11 Big Sur (Intel, amd64)

## Что сделано

### 1. Сборка (`build/build_darwin.sh`)
Добавлен внешний линкер с пробросом deployment target в общий `LDFLAGS`
(покрывает все BUILD_TYPE — universal/intel/arm64/catalina):

```sh
LDFLAGS="$LDFLAGS -linkmode=external -extldflags=-mmacosx-version-min=$MIN_MACOS_VERSION"
```

Проверка: `bash build/build_darwin.sh intel` → `.app` с `minos 11.0`
(`otool -l … | grep minos`).

### 2. Фикс TLS-краша
- `core/tls_roots_darwin.go` (новый, build-тег darwin): в `init()` грузит
  корни из `/System/Library/Keychains/SystemRootCertificates.keychain` и
  `/Library/Keychains/System.keychain` через `/usr/bin/security`, ставит
  `tls.Config{RootCAs}` на `defaultSharedTransport` и на клон
  `http.DefaultTransport`.
- `core/tls_roots_other.go` (новый, `!darwin`): no-op `initDarwinTLSRoots()`.
- `core/network_utils.go`: вызов `initDarwinTLSRoots()` из `CreateHTTPClient`.

### 3. Высота визарда
- `ui/configurator/wizard_size_darwin.go` (новый): `clampWizardSize` —
  потолок высоты окна `600` на macOS.
- `ui/configurator/wizard_size_other.go` (новый): заглушка (size as-is).
- `ui/configurator/configurator.go`: `wizardWindow.Resize(clampWizardSize(...))`.
- `ui/configurator/tabs/scroll_height.go` (новый): `adaptiveScrollSize` —
  min-высота как доля высоты окна с фолбэком.
- `ui/configurator/tabs/source_tab.go`: Outbounds-скролл `620` → адаптив
  (0.62, fallback 440).
- `ui/configurator/tabs/settings_tab.go`: `400` → адаптив (0.5, fallback 400).

## Проверка (на живом Big Sur 11.7.11)

| Проверка | Результат |
|----------|-----------|
| `minos` бинарника | `11.0` ✅ |
| Запуск, не падает на старте | ✅ (процесс жив >50с, RSS ~126 МБ) |
| HTTPS (проверка обновлений) | ✅ лог `latest: v1.1.5`, 0× `SecTrustCopyCertificateChain` |
| «Free Community Servers» | ✅ без краша |
| Высота окна визарда | ✅ 620×628, bottom=654 < Dock (экран 1280×800) |

## Замечания

- Проверено вживую на **Intel/amd64**. Сборка для **arm64/universal** идёт тем
  же путём в скрипте; на Apple Silicon с Big Sur не тестировалось (Big Sur на
  M-series — редкий кейс), но фикс линкера и пул корней архитектурно-независимы.
- TLS-пул на darwin теперь общий для всего процесса (включая
  `http.DefaultTransport`) — это меняет источник доверенных корней с системного
  верификатора на снимок из keychain на момент старта. Для лаунчера приемлемо
  (исходящие запросы к GitHub/подпискам), пользовательские изменения trust
  store применяются при следующем запуске.
