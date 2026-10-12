# SPEC 163 — Запросы LxBox к контракту/реестру по итогам аудита §591

Статус: N (new, очередь). Тип: F. Дата: 2026-10-13. Контракт сейчас 1.1.116.

Источник: сообщение сессии LxBox («Неразобранный файл аудита», решения владельца
13.10.2026), переданное в сессию SPEC 162. Не разобрано, пункты записаны как
получены. Ответ LxBox не требуется; при бампе контракта LxBox синкнет
`app/tool/sync_contract.sh --to <sha>`.

## Пункты

1. `contract/registry/protocols/naive.json` — `emit.note` устарел: эмит пишет
   `naive+quic`, фраза «нет quic» неверна; `:443` тоже проверить.
2. `contract/registry/protocols/tailscale.json` — `refs.dart` указывает на
   старый путь `add_server_wizard/tailscale_bundle.dart`; теперь
   `lib/models/tailscale_bundle.dart`; упоминание «инъекции секций узла» снять
   (§575 LxBox).
3. `body.core` в реестре / `docs/generated/index.md` называет 1.14.1-lx.4 /
   1.14.2-lx.6 — пин LxBox сейчас v1.14.2-lx.15 (сверить по
   `app/android/libbox.version`); пин лаунчера на 13.10 — 1.14.3-lx.17.
4. `hysteria.json` — `extension: desktop` стоит; решение владельца: Hysteria v1
   на мобильной стороне не поддерживать, нужен явный код отбраковки
   («Hysteria v1 is not supported, use hysteria2») вместо общего пути «узел не
   создался».
5. `warnings.json` `detour_target_missing` / `detour_cycle_broken` /
   `detour_to_group` — решение владельца «висячий detour = fail-closed везде»
   (узел выпадает с предупреждением, не «connects directly»). Смена семантики
   контракта — только если лаунчер принимает; LxBox без контракта не трогает.
   См. память `detour-failclosed-uniform` (лаунчер уже так делает).
6. `masque.json` `network` (D-078, контракт 0.8.0) — решение владельца: старые
   ссылки с `network=` читать для совместимости, при h2→h3 — предупреждение.
   Решение контракта.
7. `tls.json` `fragment` / `record_fragment` — добавить `hysteria`,
   `hysteria2`, `tuic` в `forbidden_for` (QUIC игнорирует фрагментацию); тогда
   `fieldAllowedOn` в LxBox отсечёт их без правки кода.

## Что потребуется при реализации

Изменение `contract/registry` = бамп `contract/VERSION` + changelog + параграф в
`contract/TASKS_LXBOX.md` (§114). Пункты 5 и 6 меняют семантику — отдельно
подтвердить у владельца, что лаунчер принимает их как норму контракта.
