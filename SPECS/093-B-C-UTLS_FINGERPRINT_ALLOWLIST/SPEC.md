# SPEC 093 — Неизвестный uTLS fingerprint из подписки роняет весь конфиг

**Тип:** Bug · **Статус:** Complete · **Платформа:** все · **Версия:** v1.2.7 · **Issue:** [#100](https://github.com/Leadaxe/singbox-launcher/issues/100)

## Проблема

VPN не запускался вообще: ни одна нода не поднималась, включая те, что работали
раньше. Воспроизведено на живых логах пользователя (Windows, 2026-07-30).

### Симптомы

`sing-box.log` — единственная строка, на каждой попытке старта:

```
FATAL create service: initialize outbound[26]: unknown uTLS fingerprint: hellochrome_120
```

`singbox-launcher.log` — то же самое по кругу, каждые ~4 секунды:

```
[ERROR] RebuildConfigIfDirty: sing-box check failed: FATAL[0000] initialize outbound[26]: unknown uTLS fingerprint: hellochrome_120
```

Следом — непрерывные ошибки поллера трафика (следствие: ядро не поднялось,
Clash API недоступен):

```
[WARN] traffic poller: fetch /connections failed: Get "http://127.0.0.1:9090/connections":
dial tcp 127.0.0.1:9090: connectex: No connection could be made ...
```

### Причина

В одной ноде подписки в `fp=` лежал Go-идентификатор из библиотеки uTLS —
`fp=HelloChrome_120` — вместо короткого имени, которое ждёт sing-box. Так
генерируют ссылки некоторые провайдеры: берут константу `utls.HelloChrome_120`
из кода и пишут в share-URI как есть.

`NormalizeUTLSFingerprint` (`core/config/subscription/node_parser_transport.go`)
делала только `TrimSpace` + `ToLower`, без валидации. В `config.json` уезжало
`hellochrome_120`, а sing-box сверяет это поле с фиксированным набором
(`uTLSClientHelloID` в `common/tls/utls_client.go`) и на всём остальном
прекращает загрузку.

### Почему падал весь конфиг, а не одна нода

`initialize outbound[26]` — **не** «26-я нода не поднялась». Это отказ загрузки
конфига целиком: sing-box поднимает outbound'ы по порядку, спотыкается на 26-м
и прекращает старт. Outbound'ы 0–25, полностью исправные, не поднимались тоже.

**Одна кривая нода в подписке = VPN не запускается вообще.** Отсюда цепочка всех
симптомов: `check` не проходит → ядро не стартует → Clash API на 9090 недоступен
→ поллер трафика сыпет ошибками.

Третий случай одного класса после `?ed=N` (issue #96, v1.2.6) и REALITY `pbk`
(v1.1.7): значение из share-URI попадало в конфиг без проверки на соответствие
формату sing-box.

## Требования

1. Значение `fp=` валидируется по набору, который sing-box действительно
   принимает; источник набора — исходники ядра, не память.
2. Сырые uTLS-идентификаторы (`HelloChrome_120`) распознаются и приводятся к
   семейству браузера — такая нода должна **работать**, а не отбрасываться.
3. Нераспознанное значение деградирует конкретную ноду, а не ломает конфиг.
4. Барьер стоит и в парсере, и в эмиттере: `tls.utls` может попасть в конфиг из
   шаблона, минуя парсер.
5. Регресс issue #45 (регистр `QQ` → `qq`) не сломан.

## Решение

`NormalizeUTLSFingerprint` вместо pass-through:

1. **Allowlist** — `chrome`, `firefox`, `edge`, `safari`, `360`, `qq`, `ios`,
   `android`, `random`, `randomized`, `""`, плюс пять вариантов Chrome
   ClientHello: `chrome_psk`, `chrome_psk_shuffle`, `chrome_padding_psk_shuffle`,
   `chrome_pq`, `chrome_pq_psk`. Последние пять легко пропустить, угадывая
   список по памяти — они взяты из `uTLSClientHelloID`.
2. **Маппинг алиасов** — `hellochrome*` → `chrome`, `hellofirefox*` → `firefox`
   и т.д. Разделители (`_`, `-`, пробел) вычищаются перед сравнением префикса,
   поэтому `HelloChrome-106` тоже распознаётся. Порядок важен:
   `hellorandomized` проверяется **до** `hellorandom`, иначе `HelloRandomized`
   схлопнулся бы в `random`.
3. **Всё остальное → `""`** — вызывающий код трактует это как «нет utls-блока».
4. **Эмиттер** (`GenerateNodeJSON`) для отклонённого значения не пишет ключ
   `fingerprint` вовсе — раньше записал бы `"fingerprint":""`. sing-box без
   ключа берёт свой Chrome-hello по умолчанию.

## Критерии приёмки

- [x] `fp=HelloChrome_120` → нода работает с `fingerprint: "chrome"`.
- [x] Мусорное значение (`enabled`) не доходит до `config.json`; нода живёт.
- [x] REALITY-нода чинит fingerprint, не теряя блок `reality`.
- [x] Значение из шаблона, минующее парсер, фильтруется эмиттером.
- [x] Все выходы нормализатора приняты валидатором sing-box (проверено прогоном
      через дословную копию `uTLSClientHelloID`); исходный `hellochrome_120`
      этим же прогоном подтверждён как отвергаемый.
- [x] `go build ./...`, `go test ./core/...`, `go vet ./core/config/...` зелёные.

## Границы

Только валидация uTLS fingerprint. Поле `flow` **не трогалось**: проверено, что
оно уже фильтруется в `GenerateNodeJSON` (whitelist `""`/`xtls-rprx-vision` +
guard несовместимости с v2ray-транспортами) и покрыто
`core/config/flow_transport_test.go`. См. также SPEC 035.
