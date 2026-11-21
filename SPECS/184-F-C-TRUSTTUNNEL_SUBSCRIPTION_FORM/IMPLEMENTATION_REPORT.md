# IMPLEMENTATION REPORT — SPEC 184

**Дата:** 21.11.2026. **Контракт:** 1.1.176.

## Сделано

- Ответ подписки TrustTunnel (объект JSON) даёт узел: распаковщик `container_link` печатает объект ссылкой `tt://` по диалекту секции, происхождение узла — ссылка.
- Ссылка `tt://` с одним адресом подписки при вставке на вкладке Sources становится источником-подпиской.
- «Add server» → TrustTunnel: форма по реестру, три блока.

## Проверено

- `go build ./...`, `l10n_check --strict`.
- По именам: `TestContractCorpusBody` (2 новых кейса), `TestContractCorpusForm` (2), `TestContractCorpusURI`, `TestContractTLVEmitByteParity`, `TestSubscriptionURLOfLink`, `TestSourceKinds`, линтеры схемы и реестра.

## Не проверено

- Живой узел TrustTunnel с включённой подпиской: загрузка по адресу с Basic Auth проверена только чтением кода стандартного HTTP-клиента.
- Форма и вставка ссылки-указателя в окне лаунчера — показ владельцу.
