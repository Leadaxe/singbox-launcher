# План: Система статистики (Telemetry)

## 1. Архитектура системы

### 1.1 Три репозитория

| Репозиторий | Ответственность |
|-------------|-----------------|
| **singbox-launcher** | Генерация событий, локальное хранение, отправка на relay, opt-in/opt-out |
| **singbox-launcher-telemetry-relay** | Cloudflare Worker: валидация, allowlist, rate limiting, dispatch в GitHub |
| **singbox-launcher-telemetry** | GitHub Action: приём, агрегация, stats.json/md, опционально GitHub Pages |

### 1.2 Поток данных

```
┌─────────────────┐
│   Launcher      │  POST https://telemetry.leadaxe.dev/e
└────────┬────────┘
         ▼
┌─────────────────────────────┐
│  Cloudflare Worker (relay)  │  валидация + allowlist + rate-limit
└────────┬────────────────────┘
         │  repository_dispatch
         ▼
┌─────────────────────────────┐
│  GitHub Action              │  append event, update aggregates, stats.json/md
└─────────────────────────────┘
```

---

## 2. Клиент (singbox-launcher)

### 2.1 Модульность
- Отдельный пакет `internal/telemetry`
- Разделение: сбор событий, локальное хранение, отправка

### 2.2 Компоненты
- **EventCollector** — генерация событий (launch, first_launch, error, feature_used)
- **LocalStorage** — запись в `bin/telemetry-events.jsonl`, чтение client_id из `bin/telemetry.json`
- **Sender** — батчинг, HTTP POST на relay, retry с backoff
- **Heartbeat** — таймер раз в 12 часов, асинхронная отправка

### 2.3 Выключатель
- Переменная окружения `SINGBOX_TELEMETRY=off`
- Флаг компиляции (сборки без telemetry)
- Настройки приложения (opt-in/opt-out)

### 2.4 Интеграция
- Вызов при старте: `first_launch` или `launch`
- Вызов при ошибках: `error` с кодом из allowlist
- Вызов при действиях: `feature_used` (start_tun, wizard_completed и т.д.)
- При закрытии: опционально `session_end`

---

## 3. Cloudflare Worker (Relay)

### 3.1 Endpoint
- **Route:** `POST /e`
- **URL:** `https://telemetry.leadaxe.dev/e`

### 3.2 Обязательная функциональность
1. JSON parse и валидация структуры
2. Allowlist полей — фильтрация, отброс лишнего
3. Валидация: обязательные поля, типы, enum значения
4. Rate limiting: 1 событие/10 сек на client_id, N/мин на IP (KV или Durable Object)
5. Dispatch: `POST https://api.github.com/repos/{owner}/{repo}/dispatches`, type `telemetry`

### 3.3 Environment Secrets
- `GITHUB_TOKEN`, `GITHUB_OWNER`, `GITHUB_REPO`

### 3.4 Опционально
- HMAC подпись/проверка
- Минимальное логирование

---

## 4. GitHub Action (telemetry repo)

### 4.1 Trigger
```yaml
on:
  repository_dispatch:
    types: [telemetry]
```

### 4.2 Функциональность
1. Получить событие из `repository_dispatch`
2. Валидировать структуру
3. Append в `events/YYYY-MM-DD.jsonl` (опционально)
4. Обновить агрегаты (счётчики по event, version, os, arch, error_code, feature)
5. Сгенерировать `stats.json`, `stats.md`
6. Commit и push
7. Опционально: обновить GitHub Pages дашборд

---

## 5. Формат локального хранения

**telemetry.json:**
```json
{"client_id":"uuid","enabled":true,"prev_version":"0.7.0"}
```

**telemetry-events.jsonl:**
```jsonl
{"event":"launch","ts":1739550000,"client_id":"...","app_version":"0.8.0","os":"darwin","arch":"arm64","sent":false}
```

---

## 6. Сводка изменений

| Действие | Элемент |
|----------|---------|
| **Добавить** | пакет `internal/telemetry` |
| **Добавить** | `bin/telemetry.json`, `bin/telemetry-events.jsonl` |
| **Добавить** | диалог opt-in при первом запуске |
| **Добавить** | настройка telemetry в UI |
| **Добавить** | вызовы событий в controller, process_service, wizard |
| **Создать** | репозиторий singbox-launcher-telemetry-relay |
| **Создать** | репозиторий singbox-launcher-telemetry |
