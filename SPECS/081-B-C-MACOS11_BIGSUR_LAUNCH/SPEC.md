# SPEC 081 — Запуск на macOS 11 Big Sur (краш при старте + высота визарда)

**Тип:** Bug · **Статус:** Complete · **Платформа:** macOS (amd64 + arm64)

## Проблема

На macOS 11 Big Sur релизный `.app` с GitHub не запускался: иконка в доке
«подпрыгивала» и приложение мгновенно завершалось. Подтверждено на живом
маке (Big Sur 11.7.11, Intel).

### Причина 1 — краш на TLS (главная)

Запуск бинарника из консоли давал:

```
dyld: Symbol not found: _SecTrustCopyCertificateChain
  Referenced from: .../singbox-launcher (which was built for Mac OS X 12.0)
  Expected in: /System/Library/Frameworks/Security.framework
```

- `_SecTrustCopyCertificateChain` — системный API Apple, доступный только с
  **macOS 12 (Monterey)**. На Big Sur его в `Security.framework` нет.
- Go 1.25 в `crypto/x509` (`root_darwin.go` → `systemVerify`) использует этот
  символ при проверке TLS-сертификата. Падение происходит на **первом HTTPS**
  (проверка обновлений / загрузка списка community-серверов «Free Community
  Servers»), уже после создания окна.
- Усугубление: Go internal-линкер жёстко стампит `minos 12.0`, игнорируя
  `-mmacosx-version-min` из `CGO_LDFLAGS`, поэтому символ биндится eagerly и
  процесс падает ещё до `main()`. `LSMinimumSystemVersion=11.0` в Info.plist
  фактически не соответствовал требованиям бинарника.

### Причина 2 — окно визарда не помещается на экран

Окно «Config Wizard» занимало почти всю высоту экрана; на ноутбуках с
logical 1280×800 (типичный минимум Big Sur) нижний край с навигационными
кнопками уходил под Dock. Высоту распирал **жёсткий минимум скролла 620px**
на табе Outbounds (`source_tab.go`) — AppTabs берёт максимум по табам, и окно
не могло стать ниже ~700px, игнорируя `Resize`.

## Что НЕ помогло (проверено на Big Sur)

`-tags netgo`, `CGO_ENABLED=0` (Fyne требует cgo), `GODEBUG=x509usefallbackroots=1`,
`SSL_CERT_FILE=…` — на darwin+cgo системный пул строится через системный API
до чтения env, краш остаётся.

## Решение (нужны все три части вместе)

1. **Сборка:** внешний (Xcode) линкер + проброс deployment target →
   `minos 11.0`, символ снова lazy.
   `-ldflags="-linkmode=external -extldflags=-mmacosx-version-min=11.0"`
   (`build/build_darwin.sh`).
2. **TLS:** на darwin грузим корневые сертификаты из системных keychain через
   `/usr/bin/security` (без cgo, без SecTrust) и ставим свой `RootCAs` на
   `http.DefaultTransport` **и** `core.defaultSharedTransport`. С не-системным
   `Roots` пулом `crypto/x509` использует чистый Go-верификатор и системный
   API не вызывает. Патч и `DefaultTransport`, и shared — UI-диалоги делают
   bare `&http.Client{}` (напр. `get_free_dialog.go`), идущий мимо shared.
3. **Высота визарда:** жёсткие минимумы скроллов заменены на адаптивные
   (доля высоты окна + фолбэк), как уже сделано в табах Preview/Rules. Плюс
   `clampWizardSize` ограничивает высоту окна на macOS.

## Критерии приёмки

- [x] `.app`, собранный `build_darwin.sh`, имеет `minos 11.0` (`otool -l`).
- [x] Приложение запускается на Big Sur 11.7.11 и не падает на старте.
- [x] HTTPS-запросы работают (проверка обновлений отрабатывает: лог
      `ShowUpdatePopupIfAvailable: ... latest: v1.1.5`), 0 вхождений
      `SecTrustCopyCertificateChain` в выводе.
- [x] «Free Community Servers» открывает список без краша.
- [x] Окно визарда помещается на экран 1280×800, навигационные кнопки видны
      над Dock (измерено через accessibility: 620×628, bottom=654 < Dock).

## Файлы

- `build/build_darwin.sh` — флаги внешнего линкера в `LDFLAGS`.
- `core/tls_roots_darwin.go` (new), `core/tls_roots_other.go` (new),
  `core/network_utils.go` — пул корней.
- `ui/configurator/configurator.go`, `ui/configurator/wizard_size_darwin.go`
  (new), `ui/configurator/wizard_size_other.go` (new) — потолок высоты окна.
- `ui/configurator/tabs/scroll_height.go` (new),
  `ui/configurator/tabs/source_tab.go`, `ui/configurator/tabs/settings_tab.go`
  — адаптивные минимумы высоты.
