# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Highlights

- **NaïveProxy (`naive+https://` / `naive+quic://`) support** in subscription parser and share-URI encoder. Parses user/pass, `extra-headers`, and QUIC vs HTTP/2 transport per the de-facto DuckSoft URI spec; emits sing-box `"type": "naive"` outbound with TLS block limited to `server_name` (matching sing-box naive capabilities). `padding=` URI param is logged and ignored (no sing-box equivalent). Right-click → Copy link round-trips back to a valid URI (keys in `extra-headers` sorted lexicographically for determinism). Requires sing-box ≥ 1.13.0 built with `with_naive_proxy`. See [SPEC 044](../../SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md) and `docs/ParserConfig.md` → **NaïveProxy**.

### Technical / Internal

-

---

## RU

### Основное

- **Поддержка NaïveProxy (`naive+https://` / `naive+quic://`)** в парсере подписок и share-URI энкодере. Парсятся user/pass, `extra-headers` и выбор транспорта (HTTP/2 vs QUIC) по де-факто спеке DuckSoft; собирается sing-box outbound `"type": "naive"` с TLS-блоком только `server_name` (ровно то, что поддерживает sing-box naive). Параметр `padding=` логируется и игнорируется (нет соответствия в sing-box). ПКМ → «Copy link» корректно round-trip'ит обратно в валидный URI (ключи `extra-headers` сортируются лексикографически). Требует sing-box ≥ 1.13.0 со сборкой `with_naive_proxy`. См. [SPEC 044](../../SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md) и `docs/ParserConfig.md` → **NaïveProxy**.

### Техническое / Внутреннее

-
