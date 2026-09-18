# SPEC 131 W1 · коды и сторожевые тесты — заявка от шага 2 (слой правил)

Шаг 2 волны W1 (`on_invalid` / `normalize` / `maps_to` в реестре) закончен:
**103** поля тела получили `on_invalid`, **196** параметров `uri.*` — `maps_to`,
**5** полей помечены `decision_pending`.
`warnings.json` этим шагом **не правился** — он у соседнего агента.

---

## 1. Новые коды — НЕ ТРЕБУЮТСЯ

Все 17 кодов, на которые ссылаются `on_invalid` в `body`-секциях, уже
объявлены в `contract/registry/warnings.json`:

```
anytls_min_idle_invalid   awg3_field_invalid          awg_header_invalid
field_missing             flow_deprecated             masque_vhttp_invalid
obfs_unknown              packet_encoding_unknown     port_invalid
reality_key_share_invalid reality_pbk_invalid         reality_short_id_invalid
tuic_congestion_invalid   tuic_udp_relay_mode_invalid type_invalid
utls_fp_unknown           xhttp_param_reset
```

Список исключений в линтере (`core/config/registry_body_test.go`,
`registryPendingCodes`) заведён и **пуст** — заготовка на случай, если
решения §7.10/§7.2 приведут новые коды.

### 1.1 Код, который понадобится, только если владелец выберет вариант А §7.10

| Код | Severity | Params | Desc | Где ставится |
|---|---|---|---|---|
| `ss_method_legacy` | `info` | `method` | Шифр Shadowsocks из legacy-набора `shadowstream` (`aes-*-ctr`, `aes-*-cfb`, `rc4-md5`, `chacha20-ietf`, `xchacha20`). Ядро его принимает (все 9 — вердикт D на пине 1.14.1-lx.4, DRIFT §9.3), но поток слабо защищён: это потоковые шифры без аутентификации. Узел живёт. | `shadowsocks.json` `body.fields.method` — вместо `on_invalid`, как второе правило рядом с enum'ом (значение из набора, но из legacy-подмножества) |

Заводить его **до решения владельца не нужно**: сегодня 9 legacy-шифров
дропают узел целиком, и пока правило не утверждено, `on_invalid` у `method`
не поставлен (поле помечено `"decision_pending": "SPEC 131 §7.10"`).

---

## 2. Тесты, ждущие кода

Формулировка задачи предполагала, что расширение `allowlists.json` по
ss/vmess уронит сторожевые тесты. **Не уронило, и это само по себе находка.**

Прогон: `go test ./core/config/subscription/ -run 'Registry' -count=1` → `ok`.
Прогон: `go test ./core/config/ -run 'Registry' -count=1` → `ok`.

Причина: `core/config/subscription/registry_sync_test.go` сверяет с Go-кодом
**только три** allowlist-а — `utls_fingerprints`
(`TestRegistrySyncUTLSFingerprints`), `hysteria2_obfs`
(`TestRegistrySyncHysteria2Obfs`) и `tuic_congestion`
(`TestRegistrySyncTuicCongestion`). Для `ss_methods`, `vmess_security`,
`packet_encoding`, `tuic_udp_relay_mode` и `vless_flow` сторожа **нет вообще**,
поэтому реестр и Go разъезжаются молча — ровно тот класс дыры, из-за
которого заведён сам `registry_sync_test.go` (его шапка это и описывает).

Расхождения, которые сейчас никем не ловятся и которые код догоняет в W2:

| Allowlist | Реестр (после шага 2) | Go сегодня | Последствие |
|---|---|---|---|
| `ss_methods` | 18 методов ядра (DRIFT §9.3) | 9 (`node_parser_ss.go:6-21`) | 9 рабочих legacy-узлов дропаются без объяснения |
| `vmess_security` | набор ядра, `aes-128-cfb` есть, `aes-128-ctr` нет | 6 значений с `aes-128-ctr`, без `cfb` (`node_parser_vmess.go:18-31`); Xray-ветка 5 без обоих (`xray_protocols.go:368-377`) | `ctr` из подписки = **весь конфиг мёртв** (B); `cfb` молча уезжает в `auto` |

**Заявка на W2 (или на хотфикс раньше SPEC 131 — DRIFT §7.19 ставит эти два
пункта приоритетами 🔴1 и 🔴2):**

1. Завести `TestRegistrySyncSSMethods` и `TestRegistrySyncVMessSecurity` по
   образцу трёх существующих (`checkAllowlist`), и в том же коммите
   привести Go-константы к реестру.
2. `TestRegistryAllowlistsRejectOutsiders` дополнить `isValidShadowsocksMethod`
   и `normalizeVMessSecurity`.
3. Пока сторожей нет, расхождение фиксируется только этим файлом.

---

## 3. Заявки в `warnings.json` соседнему агенту (правки текста, не новые коды)

Протухшие формулировки DRIFT §4, которые живут **в `warnings.json`** и потому
шагом 2 не тронуты:

| Код | Что поправить |
|---|---|
| `awg_headers_overlap` | «ядро отвергает такой endpoint **на загрузке**» неверно: `check` **проходит**, ошибка `headers must not overlap` приходит из `submodules/wireguard-go/device/uapi.go:839` при конфигурировании устройства, то есть **на старте** (вердикт B, но не на `check`) |
| `ss_method_invalid` | текст описывает дроп узла на 9 значениях; после решения §7.10 набор — 18 методов ядра, а дроп остаётся только для по-настоящему неизвестных |
| `utls_fp_unknown` | `desc` говорит «решения нет, нужно единое fallback-значение» — решение есть (DRIFT §2(c)): `coerce` в `chrome` + код на всех путях; реестр уже приведён |
| `reality_short_id_invalid` | `desc` описывает «фильтруется до hex-цифр/усекается» — усечения нет ни у кого, обе стороны сбрасывают в `""` (DRIFT §4) |
| `masque_vhttp_invalid` | текст (coerce в `h3`) корректен, но стоит добавить вторую ловушку с пина: `profile:"standard"` **без `uri`** даёт B при ЛЮБОМ `vhttp` (DRIFT §9.4), а не только на паре `h2 + standard` |
| `packet_encoding_unknown` | если в `desc` осталась «паника ядра (SPEC 049)» — на пине это аккуратный фатал B `unknown packet encoding`; фатальность сохранилась, паники нет |
