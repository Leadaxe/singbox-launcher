# TASKS 173

## Этап 0 — контракт
- [x] T0 Контракт 1.1.135 (D-139): `BACKUP.md` §9 п. 5, `dns.template_catalog`, три кода в `backup_warnings.json`, схема; лаунчер терпит ключ (`file_keys_10.go`), коды в `pending` теста реестра; `TASKS_LXBOX.md` §132

## Этап 1 — данные и миграция
- [x] T1 `core/state/dns_options.go`: признак посева и предложенные теги; клон
- [x] T2 `template.RecordVarDeclsFor`: порядок тегов каталога
- [x] T3 Миграция: посев каталога в старые состояния, идемпотентно

## Этап 2 — сборка и Конфигуратор
- [x] T4 `core/build/resolve_dns.go`: эмиссия шаблонного сервера по записи списка (`required` — всегда)
- [x] T5 `reconcileDNSServers`: только непредложенные теги; `syncDNSServersOnly` без доливки удалённых
- [x] T6 `DNSServerHolders` в `core/state`: удерживающие ссылки, `used_by` по контракту
- [x] T7 Вкладка DNS: Delete у шаблонных, «Add from template», «Add all defaults», «Used by …»; строки в `locale.T` + `bin/locale/ru.json`

## Этап 3 — бэкап
- [x] T8 Экспорт: `template_catalog`, полный список
- [x] T9 Импорт: снятие по §9 п. 5, `backup_dns_entry_kept`, `backup_dns_resolver_reset`, `backup_dns_group_empty`; снять коды с `pending`
- [x] T10 Кейс корпуса `v10_dns_template_catalog`, раннер по имени

## Этап 4 — закрытие
- [x] T11 `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`; показ владельцу; CI по `ci.yml`
