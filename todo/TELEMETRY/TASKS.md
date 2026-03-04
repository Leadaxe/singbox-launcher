# Задачи: Система статистики (Telemetry)

## Этап 1: MVP (клиент)

- [ ] Создать пакет `internal/telemetry`
- [ ] Реализовать генерацию `client_id` и хранение в `bin/telemetry.json`
- [ ] Реализовать события `first_launch`, `launch`
- [ ] Реализовать локальное накопление в `bin/telemetry-events.jsonl`
- [ ] Реализовать heartbeat раз в 12 часов (отправка батчей)
- [ ] Реализовать отправку на relay endpoint
- [ ] Добавить opt-in диалог при первом запуске
- [ ] Добавить возможность просмотра локально накопленных событий
- [ ] Интегрировать вызовы `launch`/`first_launch` при старте приложения

## Этап 2: Relay и Telemetry repo

- [ ] Создать репозиторий `singbox-launcher-telemetry-relay`
- [ ] Реализовать Cloudflare Worker: allowlist, валидация, rate limiting
- [ ] Реализовать dispatch в GitHub
- [ ] Создать репозиторий `singbox-launcher-telemetry`
- [ ] Реализовать GitHub Action: приём, агрегация, stats.json/md

## Этап 3: Расширение (клиент)

- [ ] Добавить события `error`, `feature_used`
- [ ] Интегрировать вызовы в process_service, wizard
- [ ] Реализовать rate limiting для ошибок на клиенте
- [ ] Добавить настройку telemetry в UI (включить/выключить)
- [ ] Реализовать retry с экспоненциальным backoff

## Этап 4: Оптимизация

- [ ] Добавить `session_end` при graceful shutdown
- [ ] Ограничить размер файла событий (ротация/архивация)
- [ ] Опционально: GitHub Pages дашборд
- [ ] Документация PRIVACY.md

## Ручное тестирование

- [ ] Первый запуск — диалог opt-in, событие first_launch
- [ ] Последующие запуски — событие launch
- [ ] Heartbeat — накопленные события отправляются
- [ ] Opt-out — события не отправляются
- [ ] Просмотр локальных событий — корректное отображение
