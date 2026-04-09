# 034 — HTTP(S) через переменные окружения (прокси) для исходящих запросов лаунчера

| Поле | Значение |
|------|----------|
| **Статус** | **Complete (C)** — закрыта 2026-04-09 |
| **Тип** | F (feature, сеть / корпоративные среды) |
| **Референс** | [PR #57](https://github.com/Leadaxe/singbox-launcher/pull/57) (upstream), локальный merge |

---

## 1. Цель

Исходящие HTTP(S)-запросы лаунчера (подписки, SRS, локали, шаблон визарда с дашборда) должны **уважать системные настройки прокси через переменные окружения** (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` и варианты с нижним регистром — поведение стандартной библиотеки Go `http.ProxyFromEnvironment`).

Это нужно пользователям за **корпоративным прокси**, где прямой выход в интернет закрыт, а браузер и другие приложения уже работают через proxy.

---

## 2. Проблема до изменений

- Общий клиент `core.CreateHTTPClient` не задавал `Transport.Proxy`, трафик шёл напрямую.
- Часть кода использовала `http.DefaultClient` или «голый» `&http.Client{Timeout: …}` без прокси и без единых таймаутов/транспорта.
- Сообщения об ошибках сети могли содержать **учётные данные в URL** (`scheme://user:password@host`), если прокси или целевой URL так форматировались.

---

## 3. Требования

### 3.1 Функциональные

1. **Единая фабрика** `CreateHTTPClient(timeout)` создаёт клиент с `http.ProxyFromEnvironment` и согласованными таймаутами/лимитами транспорта (как в `core/network_utils.go`).
2. **Подписки** — через уже существующую инъекцию `subscription.CreateHTTPClientFunc` (устанавливается в `NewConfigService`).
3. **SRS** — через `services.CreateHTTPClientFunc` (устанавливается в `NewConfigService`); при отсутствии инъекции fallback-клиент тоже должен использовать прокси из окружения (тесты, ранние пути).
4. **Локали** — через `locale.CreateHTTPClientFunc` (устанавливается в `NewAppController`); fallback — с прокси из окружения; в редком пути `GetController` fallback — та же привязка, что и при нормальном старте.
5. **Скачивание шаблона с Core Dashboard** — HTTP только через **контроллер** (`AppController.GetURLBytes`), без прямого вызова `CreateHTTPClient` из UI ([CONSTITUTION.md § 1.5](../CONSTITUTION.md) — сеть через контроллер/сервисы).
6. **Маскировка секретов в пользовательских сообщениях об ошибках сети** — `GetNetworkErrorMessage` не должен отдавать пароль из URL в открытом виде (регулярная замена на `***`).

### 3.2 Вне scope (намеренно)

- **Clash/Mihomo API** (`api/clash.go`) — запросы к локальному API; прокси из ENV для них **не включаем**, чтобы не направлять `127.0.0.1` через корпоративный прокси при неверном `NO_PROXY` (риск поломки delay/прокси-листа).
- **Парсер конфигов** — по конституции без сети; не затрагивается.

### 3.3 Нефункциональные

- Не ослаблять таймауты по сравнению с предыдущей политикой для каждого сценария.
- Тесты: unit-тесты на маскировку в пакете `internal/urlredact` (без линковки с GUI/`core` целиком).

---

## 4. Критерии приёмки

- [x] За прокси (`HTTP_PROXY`/`HTTPS_PROXY`): реализовано через `http.ProxyFromEnvironment` на общем клиенте и fallback’ах; полный ручной прогон за корпоративным прокси не обязателен для закрытия.
- [x] В сообщениях `GetNetworkErrorMessage` пароли в URL маскируются (`internal/urlredact`).
- [x] Загрузка шаблона с дашборда — только `AppController.GetURLBytes`.
- [x] `go test ./internal/urlredact/...` проходит.

---

## 5. Код-ревью (сводка)

| Тема | Вердикт |
|------|---------|
| `Proxy: http.ProxyFromEnvironment` в `CreateHTTPClient` | Корректно, стандарт для Go. |
| Инъекция `CreateHTTPClientFunc` в subscription / services / locale | Приемлемый компромисс без циклов импорта; дублирование переменных — задокументировано. |
| `urlredact.RedactURLUserinfo` | Полезно; возможны ложные срабатывания на не-URL тексте — приемлемо для error string; IPv6/экзотические URL — граничные случаи. |
| Fallback в `subscription/fetcher` без прокси | Исправлено: минимальный транспорт с `ProxyFromEnvironment`. |
| Fallback SRS / locale без прокси | Исправлено. |
| `GetController` fallback без `locale.CreateHTTPClientFunc` | Исправлено. |
| Dashboard + `core.CreateHTTPClient` | Нарушало формулировку «сеть через контроллер» — исправлено через `GetURLBytes`. |

---

## 6. Связанные файлы

| Файл | Роль |
|------|------|
| `core/network_utils.go` | `CreateHTTPClient`, `GetNetworkErrorMessage`, `GetURLBytes` |
| `internal/urlredact` | `RedactURLUserinfo` + тесты |
| `core/controller.go` | `NewAppController` → `locale.CreateHTTPClientFunc`; `GetController` fallback; `GetURLBytes` |
| `core/config_service.go` | Привязка subscription + services к `CreateHTTPClient` |
| `core/config/subscription/fetcher.go` | Fallback-клиент с прокси |
| `core/services/srs_downloader.go` | Инъекция + fallback с прокси |
| `internal/locale/locale.go` | Инъекция + fallback с прокси |
| `ui/core_dashboard_tab.go` | `GetURLBytes` с контроллера |

---

## 7. Риски

- Неверный `NO_PROXY` у пользователя — часть трафика может пойти не туда; это ответственность конфигурации окружения.
- Прокси, требующий отдельной аутентификации NTLM/Kerberos без переменных — вне scope (стандартный `ProxyFromEnvironment`).
