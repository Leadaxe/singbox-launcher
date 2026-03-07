# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Содержимое перенесено в [0-8-2.md](0-8-2.md).**

---

## EN

### Highlights
(пункты для следующего релиза после 0.8.2)

**Parser & config (WireGuard, endpoints)**

- Parser: `wireguard://` links supported as direct links. Full URI parsing: private key from userinfo, query params (publickey, address, allowedips, mtu, keepalive, presharedkey, listenport, etc.); `listen_port` only when non-zero; plus sign in keys preserved in query. WireGuard nodes are emitted into the endpoints section (sing-box 1.11+).
- Config: endpoints section for WireGuard nodes. OutboundGenerationResult has EndpointsJSON; wizard model has GeneratedEndpoints and OutboundStats.EndpointsCount. Config build writes generated endpoints between markers @ParserSTART_E / @ParserEND_E; updater writes both outbounds and endpoints when updating from subscriptions. Template includes `"endpoints": []`.
- Docs: ParserConfig.md documents WireGuard in connections, wireguard:// format, endpoints markers, and sing-box 1.11+ requirement.

**Wizard (UX & behaviour)**

- Wizard: preview cache is invalidated when sources or ParserConfig change (Add/Del source, prefix, configurator apply, manual JSON, load state).
- Wizard: View window for a source now uses the existing preview cache when available (instant after Refresh).
- Wizard: Get free VPN — confirmation dialog before applying ("Applying this configuration will replace your current sources and rules. Continue?").
- Wizard: Add/Edit Outbound dialog — when switching from Raw tab to Settings tab, form fields (tag, type, comment, filters, etc.) are now filled from the raw JSON.
- Wizard: Outbounds configurator — applying changes (Add/Edit/Delete) updates the ParserConfig text field synchronously so that switching to another tab does not overwrite the model with stale content.
- Wizard: Outbounds configurator — when editing an outbound, Scope can now be changed; the outbound is moved from the old scope (global or source) to the new one on Save.

**Wizard (model as source of truth, refactor)**

- Wizard: configurator and edit dialog no longer take ParserConfig by reference; they read from the model (presenter.Model()). Sources list and Outbounds tab callbacks also read the model at call time so the model is the single source of truth.
- Wizard: UIUpdater interface now includes Model(); business layer (ParseAndPreview, ApplyURLToParserConfig, AppendURLsToParserConfig) takes UIUpdater and gets the model from it. ModelUpdater type removed.
- Wizard: tag prefix for new sources is a single numeric index (1:, 2:, 3:, …) shared across subscriptions and connection-only blocks. On Append, new proxies get indices continuing after existing ones (e.g. 4:, 5: when three already exist).
- Wizard: proxy list building unified — one type (proxyInput) and one function (buildProxiesFromInputs) for both subscriptions and connection block; createSubscriptionProxies and matchOrCreateConnectionProxy removed. Apply and Append use the same builder with startIndex and skipConnectionsIfIn.
- Wizard/parser: dead code removed (countConnectionOnlyProxies); isConnectionOnlyProxy() used for connection-only detection; "Not preserving other connection ProxySources" log only in Apply mode.

**Fixes**

- Wizard: fixed sources disappearing after reopening wizard, changing prefixes, and saving. ParseAndPreview no longer applies the URL field to ParserConfig (which was replacing all proxies with a single source); preview and save use only ParserConfig.Proxies as the source of truth. TriggerParseForPreview no longer requires the URL field to be non-empty.

---

## RU

### Основное
(пункты для следующего релиза после 0.8.2)

**Парсер и конфиг (WireGuard, endpoints)**

- Парсер: ссылки `wireguard://` поддерживаются как прямые. Полный разбор URI: приватный ключ из userinfo, query-параметры (publickey, address, allowedips, mtu, keepalive, presharedkey, listenport и др.); `listen_port` только при ненулевом значении; плюс в ключах сохраняется. Ноды WireGuard попадают в секцию endpoints (sing-box 1.11+).
- Конфиг: секция endpoints для нод WireGuard. В OutboundGenerationResult добавлен EndpointsJSON; в модель визарда — GeneratedEndpoints и OutboundStats.EndpointsCount. Сборка конфига записывает сгенерированные endpoints между маркерами @ParserSTART_E / @ParserEND_E; updater при обновлении из подписок пишет и outbounds, и endpoints. В шаблон добавлена секция `"endpoints": []`.
- Документация: в ParserConfig.md описаны WireGuard в connections, формат wireguard://, маркеры endpoints и требование sing-box 1.11+.

**Визард (UX и поведение)**

- Визард: кеш превью сбрасывается при изменении источников или ParserConfig (добавление/удаление источника, префикс, конфигуратор, ручной JSON, загрузка состояния).
- Визард: окно View по источнику использует кеш превью при наличии (мгновенно после Refresh).
- Визард: Get free VPN — диалог подтверждения перед применением («текущие источники и правила будут заменены»).
- Визард: диалог Add/Edit Outbound — при переключении с вкладки Raw на Settings поля формы (тег, тип, комментарий, фильтры и т.д.) подтягиваются из введённого JSON.
- Визард: конфигуратор аутбаундов — применение изменений обновляет поле ParserConfig синхронно, чтобы при переключении вкладки модель не перезаписывалась устаревшим текстом.
- Визард: конфигуратор аутбаундов — при редактировании аутбаунда Scope можно менять; при сохранении аутбаунд переносится из старого scope (global или источник) в новый.

**Визард (модель как источник истины, рефакторинг)**

- Визард: конфигуратор и диалог редактирования больше не принимают ParserConfig по ссылке; данные берутся из модели (presenter.Model()). Коллбэки списка источников и вкладки Outbounds также читают модель в момент вызова — модель единый источник истины.
- Визард: в интерфейс UIUpdater добавлен метод Model(); бизнес-слой (ParseAndPreview, ApplyURLToParserConfig, AppendURLsToParserConfig) принимает UIUpdater и получает модель из него. Тип ModelUpdater удалён.
- Визард: префикс тега для новых источников — общий числовой индекс (1:, 2:, 3:, …) для подписок и connection-only блоков. При Append новые прокси получают индексы после существующих (например 4:, 5: при трёх уже существующих).
- Визард: сборка списка прокси унифицирована — один тип (proxyInput) и одна функция (buildProxiesFromInputs) для подписок и блока connections; createSubscriptionProxies и matchOrCreateConnectionProxy удалены. Apply и Append используют один и тот же построитель с startIndex и skipConnectionsIfIn.
- Визард/parser: удалён мёртвый код (countConnectionOnlyProxies); введена isConnectionOnlyProxy(); лог «Not preserving other connection ProxySources» только в режиме Apply.

**Исправления**

- Визард: исправлено исчезновение источников после повторного открытия визарда, смены префиксов и сохранения. ParseAndPreview больше не подставляет поле URL в ParserConfig (из-за этого все прокси заменялись одним источником); превью и сохранение используют только ParserConfig.Proxies как источник истины. Для запуска парсинга превью поле URL больше не обязательно.
