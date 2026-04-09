# 034 — План: HTTP ENV proxy

## 1. Архитектура

- **Один эталонный клиент** — `core.CreateHTTPClient(timeout)` с `http.ProxyFromEnvironment` и настроенным `http.Transport`.
- **Пакеты ниже `core` без импорта родительского `core`** (например `core/config/subscription`) — по-прежнему callback-переменные + fallback с тем же прокси, без импорта цикла.
- **UI → сеть** — только через метод контроллера `GetURLBytes`, внутри которого вызывается общая функция `core.GetURLBytes`.

## 2. Изменения по файлам

1. `core/network_utils.go` — при необходимости расширить: `GetURLBytes(ctx, url, timeout) ([]byte, int, error)`; документация к `sanitizeSensitiveURLInfo` / regexp.
2. `core/controller.go` — `func (ac *AppController) GetURLBytes(...)` делегирует в `GetURLBytes`; в fallback `GetController` — `locale.CreateHTTPClientFunc = CreateHTTPClient`.
3. `core/config/subscription/fetcher.go` — fallback `http.Client` с `Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}`.
4. `core/services/srs_downloader.go` — fallback transport + `ProxyFromEnvironment`.
5. `internal/locale/locale.go` — fallback client с `ProxyFromEnvironment`.
6. `ui/core_dashboard_tab.go` — заменить ручной `NewRequest` + `CreateHTTPClient` + `ReadAll` на `tab.controller.GetURLBytes` (сохранить `fyne.Do`, тексты ошибок, закрытие body внутри `GetURLBytes`).
7. `internal/urlredact` — вынести regexp-маскировку (чтобы `go test` не тянул весь `core`/Fyne); таблица кейсов в `urlredact_test.go`.

## 3. Порядок внедрения

1. Утилита `GetURLBytes` + тесты маскировки.  
2. Метод контроллера + правка дашборда.  
3. Fallback прокси + `GetController`.  
4. SPEC/TASKS/IMPLEMENTATION_REPORT, строка в `SPECS/README.md`.

## 4. Проверки

- `go test ./internal/urlredact/...`; полный `go test ./...` — по окружению (Fyne/CGO).
- Ручная проверка за прокси по чеклисту SPEC.
