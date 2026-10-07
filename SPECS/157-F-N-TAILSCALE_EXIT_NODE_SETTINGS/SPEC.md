# SPEC 157 — Роль узла Tailscale в Settings и JSON в поле Origin

Статус: **N** (запрос владельца 2026-10-06).
Тип: Feature (+ баг происхождения).
Связь: SPEC 122 §2.5 (конструктор Tailscale), SPEC 148 (вкладка Network, Save choice), SPEC 118 Т8 / SPEC 119 (поле Origin, Regen), контракт 1.1.87/1.1.88 (источник JSON-узла — тело).

## 1. Проблема

1. **Origin не сохраняется.** У узла Tailscale, чьё происхождение правили
   руками в поле Origin окна источника, вид происхождения записывался как
   `uri` (`setSourceOriginURI` знал только две формы: ссылка и wg-quick).
   Regen потом разбирал JSON как ссылку и падал: «Origin does not unpack:
   not a link: no scheme». Живой случай (профиль машины home, узел
   «Tailscale LexNet», 06.10.2026): `origin.kind = "uri"`, `origin.raw` —
   JSON тела с `exit_node` и `exit_node_allow_lan_access`.
2. **Роль узла не правится формой.** `exit_node`, `exit_node_allow_lan_access`
   и `advertise_exit_node` задаются только в конструкторе «Add server →
   Tailscale» при создании. У живого узла их можно было менять лишь в JSON
   (или через Save choice вкладки Network, но только `exit_node` и только
   из списка онлайн-пиров).

## 2. Решение

1. **Вид происхождения — по форме текста, одно правило на всех пишущих.**
   `subscription.OriginKindOfText`: JSON-объект/массив → `json`, блок
   wg-quick → `wg_ini`, иначе `uri`. Применяется в поле Origin окна
   источника, в чтении legacy-бэкапа (`uri` контракта) и в единственной точке
   материализации (`materializeServerForMigration`): JSON в поле ссылки
   уходит в JSON-ветку (документ узла и массив тел сводятся к первому телу).
   Regen в окне ветвится по форме текста, а не по хранимому виду — узел с
   уже испорченным `kind` чинится первым же Regen.
2. **Блок «Tailscale» на вкладке Settings** узла-сервера с телом
   `type: tailscale`: галка «Advertise this node as an exit node», строка
   «Exit node», галка «Keep local network reachable while using the exit
   node», пояснение. Пишет в тело рабочей копии сразу (как блок AWG),
   через санитайзер/эмиттер реестра; снятое поле удаляется из тела, а не
   пишется `false`/`""`. Роли взаимоисключающие — как в конструкторе: галка
   анонса гасит строку выхода и снимает `exit_node` с тела. У узла с
   источником-телом (`json`) источник правится вместе с телом.

## 3. Критерии приёмки

- A1. Узел «Tailscale LexNet» (kind `uri`, raw JSON): Regen в окне
  источника пересобирает тело, после него `origin.kind = json`.
- A2. Edit поля Origin → вставка JSON → Save без Regen: в state.json
  `origin.kind = json`.
- A3. У узла Tailscale на вкладке Settings виден блок «Tailscale»; смена
  выхода/галок видна на вкладке JSON без Save; Save пишет в state.
- A4. Галка анонса прячет строку выхода; в теле нет пары
  `advertise_exit_node` + `exit_node`.
- A5. У узла не-Tailscale блока нет.
- A6. legacy-бэкап с JSON в `uri` читается с `origin.kind = json`.
