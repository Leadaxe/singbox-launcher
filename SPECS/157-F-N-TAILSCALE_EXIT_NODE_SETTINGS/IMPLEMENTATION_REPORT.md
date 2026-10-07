# IMPLEMENTATION REPORT 157

Дата: 2026-10-06. Статус: реализовано, ждёт показа владельцу (UI).

## Причина бага (п.1 SPEC)

`setSourceOriginURI` (окно источника) различал две формы текста — ссылку и
wg-quick — и всё прочее записывал как `uri`. JSON, вставленный в поле Origin,
получал `origin.kind = "uri"`; Regen шёл в ветку ссылки
(`MaterializeServerNode(raw, nil)` → `ParseNode`) и падал на «not a link: no
scheme». Живой узел «Tailscale LexNet» в профиле машины home лежал именно в
таком виде.

## Что сделано

- **Вид по форме текста, одно правило.** `subscription.OriginKindOfText` /
  `IsJSONOriginText` (`core/config/subscription/origin_kind.go`). Его зовут:
  поле Origin (`setSourceOriginURI`), legacy-бэкап (`legacy_read_0x.go`,
  JSON в `uri` контракта) и `materializeServerForMigration` — JSON в поле
  ссылки уходит в JSON-ветку; документ узла и массив тел сводятся к первому
  телу (`nodeBodyOfJSONOriginText`). Так же лечится путь вставки записи
  (`normalizePastedNode`) и Save choice вкладки Network у узла с испорченным
  видом.
- **Лечение данных при загрузке** (`core/state/authored.go`,
  `normalizeBareBodyOrigin`): узел с `kind=uri` и JSON в raw получает
  `kind=json` и файл пересохраняется — вместе с прежней сводкой документа к
  телу. Без этого вкладка JSON спрашивала «Replace the source with JSON?»
  (судит по хранимому виду), а тело не считалось авторским.
- **Regen в окне** (`regenServerBodyFromRawText`) ветвится по форме текста, а
  не по хранимому виду: узел с `kind=uri` и JSON в raw пересобирается и
  получает `kind=json`. Документ/массив принимаются через
  `nodeBodyFromJSONInput`, как на вкладке JSON. После Regen раскладка
  Settings перерисовывается — подпись поля становится «Origin (raw JSON)».
- **Блок «Tailscale» на Settings** (`source_tailscale_edit.go`): галка
  «Advertise this node as an exit node», строка «Exit node», галка «Keep local
  network reachable…», пояснение. Пишет в тело рабочей копии сразу
  (`applyTailscaleRole` → `writeTailscaleBody`: Sanitize → Emit →
  StampBodyType, отказ санитайзера — диалог и откат формы); снятое поле
  удаляется из тела; роли взаимоисключающие (галка анонса гасит строку
  выхода и снимает `exit_node`). У узла с источником-телом источник
  правится вместе с телом (как в `WriteTailscaleExitNodeChoice`). Блок
  показывается по `type == tailscale` в теле, detour ему не мешает.
  Перечитывается после Regen и после Apply вкладки JSON (раскладка теперь
  пересобирается после каждого успешного Apply).
- **Подзаголовок в списке серверов** (`ui/servers_node_subtitle.go`,
  `tailscaleSubtitle`): `tailscale ‣ <выход>` — имя машины из живого
  статуса ядра, без статуса — первая метка `exit_node` из конфига;
  `tailscale·exit node` при `advertise_exit_node`; просто `tailscale` без
  выхода. Разделитель тот же, что у группы перед выбранным узлом.
- **Переводы:** два новых ключа в `bin/locale/ru.json`; остальные строки
  блока уже были у конструктора.

## Проверка

- `go build ./...` — зелёный.
- UI — показать владельцу (CONSTITUTION §8.1: тестов на вёрстку нет).
- Живой узел «Tailscale LexNet» (профиль home): после Regen в окне источника
  `origin.kind` становится `json`; то же даёт любое сохранение блока
  «Tailscale».

## Не сделано / за рамками

- Остальные поля узла Tailscale (ключ, hostname, маршруты, теги ACL) в
  Settings не выведены — задаются конструктором или в JSON.
- `SPECS/README.md` содержит чужую незакоммиченную строку (156) — файл не
  коммичу, строка 157 добавлена в рабочую копию.
