package linkmap

// Сверка ОВЕРЛЕЕВ эмита (TASKS_LXBOX §42 «После релиза», §33, §45.3, §47.2).
//
// Шесть правил обратного хода, принятых в реестр волной 1.1.36, до сих пор
// проверялись только косвенно — снимком рукописных эмиттеров
// (`TestEngineEmitVsSnapshot`) и общим кругом (`TestEngineEmitRoundTrip`).
// Снимок заморожен и новых кейсов не принимает, а общий круг молчит о том,
// КАКОЕ правило сработало: тело сошлось — и хорошо. Поэтому каждому оверлею
// здесь отведён свой круг `emit → parse → sanitize`, называющий правило по
// имени.
//
// Запуск:
//
//	go test ./core/config/linkmap -run TestEmitOverlays -count=1
//
// Правило на каждый подтест проверяется трижды: примитив объявлен реестром
// (иначе «исполняется» означало бы «зашито в движок»), ссылка несёт след
// именно этого правила, и круг возвращает тело.

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
)

// emitOverlayRoundTrip прогоняет тело через круг `emit → parse → sanitize`
// и возвращает собранную ссылку вместе с вернувшимся телом.
func emitOverlayRoundTrip(t *testing.T, h *emitHarness, dir string, body map[string]interface{}, label string) (string, map[string]interface{}) {
	t.Helper()
	uri, err := h.emitOne(dir, body, label)
	if err != nil {
		t.Fatalf("обратный ход отказал: %v", err)
	}
	scheme, plan, ok := selectPlanFor(h.plans, uri)
	if !ok {
		t.Fatalf("собранную ссылку не опознала ни одна секция: %s", uri)
	}
	res, err := ParseURI(plan, uri, h.reg.SingboxType(scheme), nil)
	if err != nil {
		t.Fatalf("круг не разобрался: %v\nссылка: %s", err, uri)
	}
	sr := nodeflow.SanitizeFrom(scheme, plan.Mapper.BodySource, res.Body)
	if sr.Drop != nil {
		t.Fatalf("санитайзер отверг круг: %s\nссылка: %s", sr.Drop.Code, uri)
	}
	return uri, sr.Clean
}

// requireSameBody сравнивает вернувшееся тело с исходным по канону
// сериализации (порядок ключей — body.order схемы), без tag/type.
func requireSameBody(t *testing.T, h *emitHarness, dir string, want, got map[string]interface{}, uri string) {
	t.Helper()
	scheme, _, ok := h.planForDir(dir)
	if !ok {
		t.Fatalf("плана %s нет", dir)
	}
	clean := map[string]interface{}{}
	for k, v := range want {
		if k == "tag" || k == "type" {
			continue
		}
		clean[k] = v
	}
	order := h.reg.Order(scheme)
	if g, w := canonString(got, order), canonString(clean, order); g != w {
		t.Errorf("круг потерял\n got: %s\nwant: %s\nссылка: %s", g, w, uri)
	}
}

// emitOverlayQuery достаёт query собранной ссылки.
func emitOverlayQuery(t *testing.T, uri string) url.Values {
	t.Helper()
	i := strings.Index(uri, "?")
	if i < 0 {
		return url.Values{}
	}
	tail := uri[i+1:]
	if j := strings.Index(tail, "#"); j >= 0 {
		tail = tail[:j]
	}
	q, err := url.ParseQuery(tail)
	if err != nil {
		t.Fatalf("query собранной ссылки не разобралась: %v (%s)", err, uri)
	}
	return q
}

// planEntryByMapsTo ищет запись плана по пути тела, в который она пишет.
// План — развёрнутая секция: именно в нём живут записи include-блоков
// (`transports#uri`, `dialer#uri`), которых в `Mapper.Params` нет.
func planEntryByMapsTo(plan *Plan, path string) *registry.Param {
	for _, group := range [][]Entry{plan.Selectors, plan.Rest} {
		for _, e := range group {
			if e.Param != nil && e.Param.MapsTo != nil && e.Param.MapsTo.Path == path {
				return e.Param
			}
		}
	}
	return nil
}

// emitSpecFor — секция обратного хода схемы.
func emitSpecFor(t *testing.T, set *registry.MapperSet, scheme string) *registry.EmitSpec {
	t.Helper()
	m, ok := set.Mapper(scheme, "uri")
	if !ok {
		t.Fatalf("секции uri у %s нет", scheme)
	}
	if m.Emit == nil {
		t.Fatalf("у %s нет emit — обратного хода реестр не объявил", scheme)
	}
	return m.Emit
}

func TestEmitOverlays(t *testing.T) {
	h := newEmitHarness(t)
	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}

	// ws.eh — `round_trip: false` у записи блока `uri/ws`.
	//
	// Имя заголовка early data в ссылку НЕ пишется: режим включается по
	// `max_early_data > 0`, а имя подставляется конвенцией на хвосте пути.
	// Круг узла с конвенциональным именем обязан вернуть тело целиком — из
	// хвоста пути, а не из query.
	t.Run("ws_eh_round_trip_false", func(t *testing.T) {
		_, plan, ok := h.planForDir("vless")
		if !ok {
			t.Fatal("плана vless нет")
		}
		rec := planEntryByMapsTo(plan, "transport.early_data_header_name")
		if rec == nil {
			t.Fatal("записи eh в плане vless нет — блок ws#uri не развернулся")
		}
		if rec.RoundTrip == nil || *rec.RoundTrip {
			t.Fatal("реестр не объявил round_trip: false у eh — правило исполнялось бы кодом")
		}
		body := map[string]interface{}{
			"type":        "vless",
			"server":      "example-1.com",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"transport": map[string]interface{}{
				"type":                   "ws",
				"path":                   "/ws",
				"max_early_data":         2048,
				"early_data_header_name": "Sec-WebSocket-Protocol",
			},
		}
		uri, got := emitOverlayRoundTrip(t, h, "vless", body, "eh")
		if q := emitOverlayQuery(t, uri); q.Get("eh") != "" {
			t.Errorf("имя заголовка уехало в query, хотя round_trip: false: %s", uri)
		}
		requireSameBody(t, h, "vless", body, got, uri)
	})

	// ss padding — `emit.userinfo.padding: false` (контракт 1.1.53).
	//
	// Эталон SIP002 пишет userinfo base64url БЕЗ «=»-паддинга, и вид
	// ссылки принадлежит формату схемы, а не удобству писателя: с 1.1.53
	// паддинг снят у обеих сторон (ревизия зеркала §3 п.6). ЧТЕНИЕ обеих
	// форм при этом обязательно и остаётся — см. корпус
	// sip002_escaped_padding / sip002_userinfo_no_padding. Написание
	// объявлено данными, чтобы ни одна сторона не поменяла своё молча.
	// `aes-128-gcm:testpass123` — 23 байта, то есть ровно один символ
	// паддинга, который здесь обязан ОТСУТСТВОВАТЬ.
	t.Run("ss_userinfo_padding", func(t *testing.T) {
		em := emitSpecFor(t, set, "ss")
		if em.UserInfo == nil || em.UserInfo.Padding == nil {
			t.Fatal("реестр ss не объявил emit.userinfo.padding — написание решал бы код")
		}
		if *em.UserInfo.Padding {
			t.Fatal("ожидался padding: false (SIP002, контракт 1.1.53)")
		}
		body := map[string]interface{}{
			"type":        "shadowsocks",
			"server":      "example-1.com",
			"server_port": 8388,
			"method":      "aes-128-gcm",
			"password":    "testpass123",
		}
		uri, got := emitOverlayRoundTrip(t, h, "shadowsocks", body, "ss")
		if strings.Contains(uri, "=@") {
			t.Errorf("userinfo с «=»-паддингом, хотя padding: false: %s", uri)
		}
		requireSameBody(t, h, "shadowsocks", body, got, uri)
	})

	// Булев БЕЗ `emit_as` — умолчание пишет СЛОВОМ (PRIMITIVES §0.12a,
	// контракт 1.1.53, ревизия зеркала §3 п.3).
	//
	// Три поля xhttp объявлены `bool_spelled` и `emit_as` НЕ несут: их
	// написание на выходе решает умолчание, а не запись. Go писал словом,
	// Dart цифрой, и расхождение компенсировалось оверлеем `emit_as: raw`
	// у этих же трёх полей — оверлей снимается, умолчание закреплено
	// здесь и кейсом корпуса uri/vless/xhttp_bool_default_spelled.
	//
	// Подтест судит УМОЛЧАНИЕ, поэтому первым делом требует, чтобы
	// `emit_as` у полей и правда отсутствовал: появись он, правило стало
	// бы объявленным, и проверять умолчание было бы нечем.
	t.Run("bool_default_spelled", func(t *testing.T) {
		fields := []string{"noGRPCHeader", "noSSEHeader", "xPaddingObfsMode"}
		// Записи приезжают из блока transports#uri, подключённого vless
		// через `include`, и разворачивает их ПЛАН, а не секция: у
		// registry.Mapper в Params лежат только записи самой схемы.
		_, plan, ok := h.planForDir("vless")
		if !ok {
			t.Fatal("плана vless#uri нет")
		}
		seen := map[string]bool{}
		for _, e := range plan.Rest {
			for _, name := range fields {
				// Запись блока несёт имя ПОДБЛОКА: `xhttp.noGRPCHeader`.
				if e.Name != name && e.Name != "xhttp."+name {
					continue
				}
				seen[name] = true
				if e.Param.EmitAs != "" {
					t.Fatalf("у %s объявлен emit_as %q — подтест судит УМОЛЧАНИЕ",
						name, e.Param.EmitAs)
				}
			}
		}
		for _, name := range fields {
			if !seen[name] {
				t.Fatalf("записи %s в плане vless#uri нет", name)
			}
		}
		body := map[string]interface{}{
			"type":        "vless",
			"server":      "example-1.com",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			// server_name и utls в теле — то, что вернёт круг: ссылка
			// несёт security=tls, а SNI по умолчанию равен адресу
			// сервера (default_from), fp — `random`. Без них подтест
			// сравнивал бы тело с ДООПРЕДЕЛЁННЫМ телом и падал не на
			// написании булева, а на этих двух полях.
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "example-1.com",
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "random",
				},
			},
			"transport": map[string]interface{}{
				"type":                "xhttp",
				"path":                "/xh",
				"mode":                "auto",
				"no_grpc_header":      true,
				"no_sse_header":       true,
				"x_padding_obfs_mode": true,
			},
		}
		uri, got := emitOverlayRoundTrip(t, h, "vless", body, "xhttp-bool")
		q := emitOverlayQuery(t, uri)
		for _, name := range fields {
			switch v := q.Get(name); v {
			case "true":
				// Умолчание сработало.
			case "1", "0":
				t.Errorf("%s уехал ЦИФРОЙ (%q) — умолчание §0.12a пишет словом: %s", name, v, uri)
			case "":
				t.Errorf("%s в ссылку не уехал вовсе: %s", name, uri)
			default:
				t.Errorf("%s уехал как %q, ожидалось слово true: %s", name, v, uri)
			}
		}
		requireSameBody(t, h, "vless", body, got, uri)
	})

	// vmess json_map — форма-КОНТЕЙНЕР, а не query-ссылка; json_always —
	// ключи, которые панели ждут даже пустыми.
	t.Run("vmess_json_map", func(t *testing.T) {
		em := emitSpecFor(t, set, "vmess")
		if len(em.JSONMap) == 0 {
			t.Fatal("реестр vmess не объявил emit.json_map — контейнер собирал бы код")
		}
		if len(em.JSONAlways) == 0 {
			t.Fatal("реестр vmess не объявил emit.json_always — ключи-заполнители решал бы код")
		}
		body := map[string]interface{}{
			"type":        "vmess",
			"server":      "example-1.com",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"security":    "auto",
		}
		uri, got := emitOverlayRoundTrip(t, h, "vmess", body, "vmess")
		if !strings.HasPrefix(uri, "vmess://") {
			t.Fatalf("ожидалась форма-контейнер vmess://: %s", uri)
		}
		blob := strings.TrimPrefix(uri, "vmess://")
		if i := strings.Index(blob, "#"); i >= 0 {
			blob = blob[:i]
		}
		raw, err := base64.StdEncoding.DecodeString(blob)
		if err != nil {
			raw, err = base64.RawStdEncoding.DecodeString(blob)
		}
		if err != nil {
			t.Fatalf("контейнер не base64: %v (%s)", err, uri)
		}
		var container map[string]interface{}
		if err := json.Unmarshal(raw, &container); err != nil {
			t.Fatalf("контейнер не JSON: %v (%s)", err, raw)
		}
		for key := range em.JSONAlways {
			if _, has := container[key]; !has {
				t.Errorf("ключ json_always %q в контейнере отсутствует: %s", key, raw)
			}
		}
		requireSameBody(t, h, "vmess", body, got, uri)
	})

	// omit_port у naive — примитив ОБЪЯВЛЕН, но не выставлен нигде (§33.6).
	//
	// Проверка сторожевая: она падает не на отсутствии атрибута, а на его
	// ПОЯВЛЕНИИ без встречного кейса. Пока порт пишется всегда, ссылка
	// обязана его нести; если сторона назовёт схему, где порт опускается,
	// тест потребует завести кейс здесь же.
	t.Run("naive_omit_port_declared_unset", func(t *testing.T) {
		em := emitSpecFor(t, set, "naive")
		if em.OmitPort {
			t.Fatal("naive объявил emit.omit_port — заведите кейс круга на опущенный порт (§33.6)")
		}
		body := map[string]interface{}{
			"type":        "naive",
			"server":      "example-1.com",
			"server_port": 443,
			"username":    "user",
			"password":    "testpass123",
			// naive без TLS не бывает: разбор материализует блок сам, и
			// тело без него на круге «приобрело» бы поля, которых не теряло.
			"tls": map[string]interface{}{"enabled": true, "server_name": "example-1.com"},
		}
		uri, got := emitOverlayRoundTrip(t, h, "naive", body, "naive")
		if !strings.Contains(uri, "example-1.com:443") {
			t.Errorf("порт опущен, хотя omit_port не выставлен: %s", uri)
		}
		requireSameBody(t, h, "naive", body, got, uri)
	})

	// refuse_when у wireguard — ссылка выражает ОДНОГО пира.
	//
	// Круг одного пира обязан сойтись, а узел с двумя — получить отказ, а
	// не ссылку по первому. Отказ объявлен данными, а не проверкой в коде.
	t.Run("wireguard_refuse_when", func(t *testing.T) {
		em := emitSpecFor(t, set, "wireguard")
		if len(em.RefuseWhen) == 0 {
			t.Fatal("реестр wireguard не объявил emit.refuse_when — отказ решал бы код")
		}
		peer := func(addr string) map[string]interface{} {
			return map[string]interface{}{
				"address":     addr,
				"port":        51820,
				"public_key":  "MTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTE=",
				"allowed_ips": []interface{}{"0.0.0.0/0"},
			}
		}
		body := map[string]interface{}{
			"type":        "wireguard",
			"peers":       []interface{}{peer("192.0.2.1")},
			"private_key": "MjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjI=",
			"address":     []interface{}{"10.0.0.2/32"},
		}
		uri, got := emitOverlayRoundTrip(t, h, "wireguard", body, "wg")
		requireSameBody(t, h, "wireguard", body, got, uri)

		two := map[string]interface{}{}
		for k, v := range body {
			two[k] = v
		}
		two["peers"] = []interface{}{peer("192.0.2.1"), peer("192.0.2.2")}
		if uri, err := h.emitOne("wireguard", two, "wg2"); err == nil {
			t.Errorf("узел с двумя пирами выражен ссылкой вместо отказа: %s", uri)
		}
	})

	// round_trip_only: "emit" у detour — запись, у которой ЧТЕНИЯ нет.
	//
	// `detour` ставит СБОРКА конфига, а не ссылка: пришедший по ссылке
	// detour указывал бы на тег, которого в чужом конфиге нет, и узел уехал
	// бы в fail-closed. Поэтому запись объявлена эмит-онли: ссылка его
	// несёт, а круг возвращает тело БЕЗ detour.
	t.Run("detour_round_trip_only_emit", func(t *testing.T) {
		_, plan, ok := h.planForDir("trojan")
		if !ok {
			t.Fatal("плана trojan нет")
		}
		rec := planEntryByMapsTo(plan, "detour")
		if rec == nil {
			t.Fatal("записи detour в плане trojan нет — блок dialer#uri не развернулся")
		}
		if rec.RoundTripOnly != "emit" {
			t.Fatalf("ожидалось round_trip_only: emit, объявлено %q", rec.RoundTripOnly)
		}
		body := map[string]interface{}{
			"type":        "trojan",
			"server":      "example-1.com",
			"server_port": 443,
			"password":    "testpass123",
			"detour":      "next-hop",
		}
		uri, got := emitOverlayRoundTrip(t, h, "trojan", body, "detour")
		if !strings.Contains(uri, "next-hop") {
			t.Errorf("эмит-онли запись detour в ссылку не попала: %s", uri)
		}
		if _, has := got["detour"]; has {
			t.Errorf("detour вернулся ЧТЕНИЕМ, хотя запись эмит-онли: %v (ссылка %s)", got, uri)
		}
		want := map[string]interface{}{}
		for k, v := range body {
			if k != "detour" {
				want[k] = v
			}
		}
		requireSameBody(t, h, "trojan", want, got, uri)
	})

	// keep_empty_tail у socks4 — `emit.userinfo.keep_empty_tail: true`
	// (QUIRKS Q133-74, TASKS_LXBOX §47.2).
	//
	// У socks4 пароля нет по протоколу, но разделитель клиенты пишут всегда
	// (`socks4://userid:@host`), и его отсутствие часть из них читает как
	// «имени нет». Явное объявление схемы сильнее общей конвенции одиночного
	// userinfo (`single_into` → `userid@host`): конвенция существует ради
	// схем, ничего не объявивших. Круг обязан вернуть тело без пароля —
	// пустой хвост на входе не материализуется.
	t.Run("socks4_keep_empty_tail", func(t *testing.T) {
		em := emitSpecFor(t, set, "socks")
		if em.UserInfo == nil || !em.UserInfo.KeepEmptyTail {
			t.Fatal("реестр socks не объявил emit.userinfo.keep_empty_tail — разделитель решал бы код")
		}
		body := map[string]interface{}{
			"type":        "socks",
			"server":      "example-1.com",
			"server_port": 1080,
			"version":     "4",
			"username":    "useridonly",
		}
		uri, got := emitOverlayRoundTrip(t, h, "socks", body, "socks4")
		if !strings.HasPrefix(uri, "socks4://useridonly:@") {
			t.Errorf("разделитель у пустого хвоста не записан, хотя keep_empty_tail: true: %s", uri)
		}
		if _, has := got["password"]; has {
			t.Errorf("пустой хвост вернулся паролем: %v (ссылка %s)", got, uri)
		}
		requireSameBody(t, h, "socks", body, got, uri)
	})
}
