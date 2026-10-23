# PLAN 173 — каталог шаблонных DNS-серверов

## Данные

- `core/state/dns_options.go`: признак посева каталога и множество предложенных тегов (`TemplateCatalogSeen []string` или эквивалент) в `DNSOptions`; клон и сравнение — рядом.
- Миграция: шаг загрузки состояния, идемпотентный по признаку посева. Вход — состояние и теги каталога шаблона с `enabled` по умолчанию (`template.RecordVarDeclsFor` уже даёт `DNSServerEnabled`; добавить порядок тегов `DNSServerOrder`). Решить, нужен ли бамп версии формата состояния (`migration_v7_to_v8.go` как образец) — если признак хранится в состоянии, достаточно идемпотентного шага.

## Сборка

- `core/build/resolve_dns.go`: библиотека шаблона эмитит сервер, только если у тега есть запись `kind: template` в списке, либо сервер `required`. Порядок — список состояния. `stateTemplateEnabled` с запасным умолчанием остаётся только для `required`.

## Конфигуратор

- `ui/configurator/business/reconcilers.go` `reconcileDNSServers`: проход 1 по каталогу дописывает только непредложенные теги (новые серверы шаблона), удалённые не возвращает.
- `ui/configurator/business/wizard_dns.go`: `DNSTagFromTemplate` больше не блокирует удаление; блок — `required` и удерживающие ссылки.
- `ui/configurator/tabs/dns_tab.go`: Delete у шаблонных, «Add from template», «Add all defaults», подпись «Used by …».
- `ui/configurator/models/preset_ref_sync.go` `syncDNSServersOnly`: из модели в состояние — ровно список модели, без доливки `explicitOverrides` по удалённым тегам.

## Удерживающие ссылки

- Новая чистая функция в `core/state` (без UI-зависимостей): `DNSServerHolders(state, templateRequired) map[tag]usedBy`. Её вызывают вкладка и `importDNS`.

## Бэкап

- `core/backup/export10.go`: `template_catalog` из `opts.RecordVars` (порядок), `dns.servers` — список как есть.
- `core/backup/import.go` `importDNS`: снятие по норме §9 п. 5, сброс резолверов, пустая группа; константы трёх кодов; `core/backup/types.go` — поле `TemplateCatalog` в разборе `dns` 1.0; `file_keys_10.go` — ключ из рефлексии, ручной `withKeys` снять.
- `core/backup/schema_test.go`: убрать три кода из `pending`.

## Тест

Один интеграционный на волну (CONSTITUTION §8.1): корпус бэкапа — кейс `v10_dns_template_catalog` (файл с каталогом: снятие, удержание `final`, тег вне каталога) в `contract/corpus/backup/`, раннер корпуса по имени.

## Документация

- `docs/ARCHITECTURE.md` (поток DNS-серверов: каталог → список → сборка), `docs/release_notes/upcoming.md` EN/RU, `RELEASE_NOTES.md` (UX: удаление шаблонных серверов).
