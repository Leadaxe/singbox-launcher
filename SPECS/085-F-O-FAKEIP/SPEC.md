# 085-F-O — FakeIP

**Тип:** Feature · **Статус:** O (backend реализован) · **Дата:** 2026-07-12
**Ядро:** `1.14.0-lx.1-rc.17` (FakeIP как DNS-server type — есть, проверено `sing-box check`).

## 1. Проблема

FakeIP ускоряет установку соединения и снижает DNS-утечки: проксируемые A/AAAA-запросы
получают синтетический IP из пула, реальный домен восстанавливается сниффингом. Частый
запрос сообщества («FakeIP должен быть в Traffic Processing», критика DNS-вкладки, июль 2026).
В лаунчере поддержки не было (только `fakeip` как значение типа сервера в UI без генерации).

## 2. Решение

FakeIP оформлен как **self-contained пресет** `fakeip` в `bin/wizard_template.json` — по той же
механике, что `russian`/`block-ads` (SPEC 053). Пользователь включает его в Routing, как любой
другой пресет. Ничего в коде для «включения» не требуется — работает через существующий
preset-expand → `core/build/resolve_dns.go`.

Состав пресета:
- **vars**: `dns_server` (тип `dns_server`, default `fakeip`), `inet4_range` (`198.18.0.0/15`),
  `inet6_range` (`fc00::/18`).
- **dns_servers**: `{tag:"fakeip", type:"fakeip", inet4_range:"@inet4_range", inet6_range:"@inet6_range"}`.
  При build tag префиксуется → `fakeip:fakeip` (как `russian:yandex_udp`).
- **dns_rule**: `{query_type:["A","AAAA"], server:"@dns_server"}` — A/AAAA проксируемых доменов
  идут в fakeip. Пресет стоит в каталоге ПОСЛЕ `russian`, поэтому RU-домены резолвятся реально
  до fakeip (порядок пресетов = приоритет правил, sing-box first-match).
- **persistence**: `experimental.cache_file.store_fakeip = true` (в базовом config); маппинг
  fakeip переживает рестарт. Флаг инертен без fakeip-сервера.

## 3. Изменения в коде

| Файл | Изменение |
|------|-----------|
| `core/template/preset_types.go` | +`Inet4Range`/`Inet6Range` в `PresetDNSServer` (были бы отброшены типизированной структурой) |
| `core/build/resolve_dns.go` | `substitutePresetDNSServer` эмитит `inet4_range`/`inet6_range` |
| `bin/wizard_template.json` | +пресет `fakeip`; `cache_file.store_fakeip:true` |
| `core/build/fakeip_test.go` | тест: сервер с диапазонами + A/AAAA-правило |

`stripDNSWizardOnlyFields` использует strip-allowlist — новые поля проходят. `MergeDNSSection`
работает с `[]json.RawMessage` — passthrough.

## 4. Проверка (DoD)

- `go test ./core/build/ ./core/template/` — **OK** (новый `TestResolveDNS_FakeIPPreset` + без регрессий).
- `go vet` — **OK**.
- **Ядро rc.17**: эмитируемая форма (`type:"fakeip"` + `inet4_range`/`inet6_range` + `store_fakeip`)
  принята `sing-box check` — **OK**.

## 5. Ограничение (follow-up)

**HTTPS/SVCB-блок** (`{query_type:["HTTPS","SVCB"], action:"predefined", rcode:"NOERROR"}`) —
не включён. Причина: пресет поддерживает **один** `dns_rule`, а блок — это второе правило с
другим action. Добавление `dns_rules` (plural) затрагивает delicate DNS-ordering машинерию
SPEC 062 (единый порядок route+DNS, уже откатывался — регрессия DNS-save `a58a176`), что
небезопасно в автономном режиме без live-GUI прогона. FakeIP без блока полностью функционален
для A/AAAA; блок — оптимизация против обхода через HTTPS RR ipv4hint. → отдельная задача
085.1 (расширить preset до `dns_rules` + UI-toggle `force`).

## 6. UI (follow-up 085.2)

Пресет уже виден в Routing как любой другой (read-only каталог → копия в правила). Отдельный
тумблер «FakeIP» в Traffic Processing и var-редактор диапазонов — по образцу существующего
preset-var editor. Больших списков не касается (нет проблемы Fyne).
