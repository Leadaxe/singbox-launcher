# SPEC 077-F-M — DETOUR-ЦЕПОЧКИ ДЛЯ SERVER / SUBSCRIPTION

## Цель

Дать пользователю в настройках **источника** (одиночный сервер `type=server` и подписка `type=subscription`) выбрать **detour-сервер** — другой адресуемый outbound, через который пойдут все узлы этого источника. Это строит proxy-цепочки (chain/hop): клиент → detour-хоп → узел → интернет.

В sing-box dial-based outbound (`vless`/`vmess`/`trojan`/`shadowsocks`/`socks`/`http`/`hysteria2`/`tuic`) поддерживает поле `"detour": "<tag>"` — соединение к этому серверу устанавливается через outbound с указанным тегом. Узел `A` с `detour: B` = «`A` через `B`».

## Контекст (что уже есть)

- **Эмиссия `detour` уже реализована** (SPEC 036, Xray dialerProxy): `node.Outbound["detour"] = tag` → `GenerateNodeJSON` пишет `"detour":"<tag>"` (`core/config/outbound_generator.go:331-339`). Новая фича переиспользует ровно этот механизм — генератор править почти не нужно.
- **`ParsedJump`** (`configtypes/types.go:197`) — per-node hop из Xray-подписки. Это **другое**: цепочка, навязанная самой подпиской. Наша фича — пользовательский detour на уровне источника.
- **Реестр тегов**: `GetAvailableOutbounds(model)` (`ui/configurator/business/outbound.go`) возвращает отсортированный список доступных outbound-тегов (глобальные + локальные группы источников + preset-группы + `direct-out`/`reject`/`drop`). Это источник вариантов для дропдауна.
- **Маппинг состояния**: `state.Source` → `configtypes.ProxySource` в `core/state/adapter_source.go` (`ToProxySourceV4`) и `core/state/sync_to_legacy.go`; обратный — `sync_to_connections.go`. Ноды источника создаются в `subscription.LoadNodesFromSource`.
- **Поля видимости** у `Source` уже есть (`ExcludeFromGlobal`, `ExposeGroupTagsToGlobal`) — detour ложится в ту же модель «настройка источника».

## Объём

**В объёме:**

1. **Модель**: новое поле `Source.DetourTag string` (state, для обоих типов) и `ProxySource.DetourTag string` (runtime); проброс в обоих направлениях (`ToProxySourceV4`, `sync_to_legacy`, `sync_to_connections`).
2. **Применение**: в `LoadNodesFromSource` для каждого узла источника с непустым `DetourTag` проставить `node.Outbound["detour"] = DetourTag` (кроме `wireguard` — см. вне объёма). Существующая эмиссия в `config.json` подхватит.
3. **Валидация (build-time, в генераторе)** перед эмиссией:
   - **самоссылка** (detour-тег = собственный тег узла) → убрать + WarnLog;
   - **цикл среди узлов** (`A.detour=B`, `B.detour=A`, и длиннее, где B,A — узлы источников) → детектировать, разорвать на одном звене + WarnLog.
   - **висячий на шаблонную/preset-группу НЕ дропается здесь** (см. дизайн-решения): на этапе генерации outbound'ов теги шаблонных групп ещё не известны (подмешиваются при финальной сборке `config.json`), поэтому дроп был бы ложным. UI предлагает только валидные теги из `GetAvailableOutbounds`; ручной несуществующий тег поймает ядро при загрузке с явной ошибкой.
4. **UI**: в `source_edit_window.go` (Settings-вкладка) для server и subscription — дропдаун «Detour server» из `GetAvailableOutbounds` (минус собственные теги источника), пункт «(none)» для сброса. Сохранение через существующий `serializeParserAfterSourceEdit` flow.
5. **Конфликт с Xray Jump**: если у узла уже есть `Jump` (подписка навязала свою цепочку), пользовательский `DetourTag` к этому узлу **не применяется** (debug-лог). Jump приоритетнее как требование самого узла.
6. **Тесты**: маппинг round-trip, генерация (server+detour, subscription+detour → `config.json` с `"detour"`), валидация (висячий/само/цикл), golden-фрагмент. **Локали** для UI-строк.
7. **Документация**: `docs/ParserConfig.md`, `docs/release_notes/upcoming.md`.

**Вне объёма:**

- **WireGuard-endpoint detour** — WG идёт в `endpoints[]`, dialer-семантика отличается; в MVP detour к wg-узлам не применяется (пропуск + debug-лог). Возможное расширение позже.
- **Detour на отдельную ноду подписки** как target — нестабильно (теги генерятся), как и отклонённый п.2 обсуждения. Target — стабильные адресуемые outbound'ы (группы, глобальные, другие серверы).
- **Множественные хопы из UI** (A→B→C вручную) — достижимо через цепочку detour у разных источников, но отдельного multi-hop-редактора не делаем.
- **Хранение ссылки по `Source.ID`** — для MVP храним detour **по тегу** (консистентно со всем кодом: правила/селекторы/`AddOutbounds` ссылаются по тегу). Нестабильность тега server-цели страхуется build-валидацией (висячий detour → fallback на прямой dial). Переход на ref-by-ID — возможный follow-up, если потребуется.

## Дизайн-решения

- **По тегу, не по ID.** Весь существующий механизм (`GetAvailableOutbounds`, `AddOutbounds`, custom_rules) работает на тегах-строках. Вводить ref-by-ID только ради detour — рассинхрон с остальным кодом. Цена — нестабильность при переименовании server-цели; снимается build-валидацией (узел продолжает работать напрямую, а не ломает конфиг).
- **Применяем ко всем узлам источника.** Семантика «вся подписка/сервер ходит через хоп X». Для одиночного server это один узел.
- **Fail-open.** Любая невалидность detour (висячий/цикл/само/wg) → поле убирается, узел работает напрямую, в лог — WarnLog. Никогда не валим генерацию конфига из-за detour.

## Критерии приёмки

1. Server-источник с выбранным detour → в `config.json` его outbound содержит `"detour":"<target>"`, target присутствует среди outbound'ов; `sing-box check` принимает.
2. Subscription-источник с detour → каждый его узел получает `"detour":"<target>"`.
3. Висячий/самоссылка/циклический detour не валит сборку: поле убрано, WarnLog, остальной конфиг валиден.
4. Узел с Xray-Jump игнорирует пользовательский detour (Jump побеждает).
5. UI: дропдаун присутствует у server и subscription, не предлагает собственные теги источника, «(none)» сбрасывает; выбор переживает закрытие/переоткрытие окна (round-trip через state).
6. `go build ./... && go test ./... && go vet ./...` зелёные.
