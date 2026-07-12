# 092-F-O — TLS anti-DPI трансформы (fragment / record_fragment / mixed-case SNI)

**Тип:** Feature · **Статус:** O (backend реализован; UI-тумблеры — follow-up) · **Дата:** 2026-07-12
**Ядро:** rc.17 — `tls.fragment`/`fragment_fallback_delay`/`record_fragment` есть (`option/tls.go`), проверено `sing-box check`.

## 1. Проблема

Топ-1 запрошенная (двумя независимыми разведочными агентами) и самая востребованная в РФ/СНГ
возможность обхода DPI/ТСПУ: расщепление TLS ClientHello, чтобы DPI не смог сматчить SNI в одном
чтении. В лаунчере отсутствовала полностью. LxBox имеет это как build-post-step
(`tls_transforms.dart`) на first-hop outbound'ах.

## 2. Решение (порт LxBox-паттерна, в стиле проекта)

Чистый build-time post-step над сгенерированными outbound'ами (`[]json.RawMessage`), применяемый
в `buildSection("outbounds")` где доступны и кэш, и `ctx.Vars`. Три opt-in трансформа:
- **`tls_fragment`** → `tls.fragment:true` (+ `fragment_fallback_delay`).
- **`tls_record_fragment`** → `tls.record_fragment:true`.
- **`tls_mixed_case_sni`** → рандомизация регистра SNI (RFC 6066 case-insensitive; сервер
  принимает, case-sensitive DPI-блоклист — нет). **Детерминированно** (FNV-хэш хоста → стабильно
  между сборками, без churn config.json и без golden-флейка); Punycode-метки (`xn--`) не трогаются.

Применяется ТОЛЬКО к first-hop TLS outbound'ам: `tls.enabled:true`, без `detour`, тип не
direct/block/dns/selector/urltest/naive. Inner-hop detour-цепочки и utility-outbound'ы не трогаются.
No-op когда ни один тумблер не включён → нетронутый config байт-в-байт прежний.

## 3. Файлы

| Файл | Изменение |
|------|-----------|
| `core/build/tls_transforms.go` (новый) | `TLSTransformOptions`, `ApplyTLSTransforms`, `isFirstHopTLSOutbound`, `mixedCaseSNI`, `TLSTransformOptionsFromVars` |
| `core/build/build.go` | вызов трансформа в `buildSection("outbounds")` перед `BuildOutboundsSection` |
| `bin/wizard_template.json` | vars `tls_fragment`/`tls_record_fragment`/`tls_fragment_fallback_delay`/`tls_mixed_case_sni` (после `cert_store`) |
| `core/build/tls_transforms_test.go` | тесты: fragment на first-hop, skip detour/utility/naive/no-tls, no-op, детерминизм+Punycode, var-парсинг |

## 4. Проверка (DoD)

- `go test ./core/build/` — OK (новые тесты + без регрессий golden).
- `go vet` — OK.
- Ядро rc.17 принимает эмит (`tls.fragment`+`record_fragment`+mixed-case SNI): `sing-box check` — OK.
- Vars появляются на вкладке Settings автоматически (существующий рендер `wizard_ui:edit` bool/text vars).

## 5. UI (follow-up 092.1)

Тумблеры уже отрисуются на Settings из `vars` (тип bool/text, `wizard_ui:edit`) существующим
механизмом. Отдельная UX-полировка (группировка «Anti-DPI», подсказки) — опционально; требует
GUI-проверки, что новые vars корректно отображаются и сохраняются.

## 6. Замечания

- naive намеренно исключён (управляет своим TLS/HTTP2-стеком — LxBox делает так же).
- Трансформ применяется к proxy-нодам подписок; для detour-цепочек — только к внешнему хопу
  (внутренние идут внутри туннеля, DPI их не видит).
