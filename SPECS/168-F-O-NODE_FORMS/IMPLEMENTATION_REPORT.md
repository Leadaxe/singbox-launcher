# IMPLEMENTATION REPORT 168

**Итог:** формы «Add server» и правки узла для VLESS, Hysteria2, Trojan, WireGuard (AWG), naive, masque; SOCKS5, HTTP, WireGuard и OpenConnect — на одной модели по реестру. Контракт 1.1.123.

## Что сделано
- Реестр: `form` у ref-полей схемы и у полей общих секций `tls`/`transports`; `form.fixed`, `form.implies`, `form.exclude_fields`, `form.exclude_variants`; массив объектов правится элементом `[0]`; активный вариант выбора — по самому длинному совпадению (OpenConnect не изменился). API: `registry.FormFields`, `FormState`, `ApplyFormFixed`, `FormValue`/`SetFormValue`/`DeleteFormValue` (`core/config/registry/form.go`).
- Записи 8 схем, `tls.json`, `transports.json` (у ws/httpupgrade `headers.Host` + `*`); корпус формы — 19 кейсов на 9 схем; `TASKS_LXBOX.md` §120; changelog (плюс пропущенная строка 1.1.122).
- UI: `node_form_model.go`, `node_form_labels.go`, `node_form_view.go`, `source_form_edit.go`; удалены ручные ветки SOCKS/HTTP/WG, сборщики URI и их тесты; 38 ключей locale с переводом.

## Решения исполнителей
- `exclude_fields` у ref-поля: у Trojan нет Reality, у HTTP — ALPN/uTLS/Reality (по `forbidden_for` им разрешены).
- masque: `fixed tls.enabled` (ядро masque включает TLS само).
- `obfs.password` у Hysteria2 обязателен, когда виден (иначе санитайзер снимает весь `obfs`).
- Тело из формы прогоняется через санитайзер и эмиттер: иначе авторское тело не получало дефолтов реестра (MTU 1280 у AWG, `allowed_ips` у WG) — проверено на сохранённом теле и сборке.
- Новый VLESS открывается с Security «None» (вариант по умолчанию реестра).
- Поля AWG3 и DNS у WireGuard — только JSON (DNS реестр игнорирует, `wgconf_dns_ignored`).

## Проверки
`go build ./...`; `l10n_check --strict`, `hardcoded_check`; `TestContractCorpusForm`, `TestRegistryBodyStructure`, `TestContractCorpusBody`, `TestRegistryCodeRefsResolve`; повторный `go generate ./contract/...` без диффа. UI вживую — показ владельцу. Полный прогон — CI.
