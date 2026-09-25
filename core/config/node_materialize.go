// File node_materialize.go — ЕДИНСТВЕННАЯ точка рождения тела узла
// (SPEC 131 W2c, §3.5).
//
// До этой волны тело узла рождалось в трёх местах и по трём разным правилам:
// per-scheme эмиттер (`GenerateNodeJSONBare`, цепочка `else if` с
// allowlist-ами полей), ручной JSON «как есть» (`stripTagAndDetour` мимо
// всякой проверки) и сборщик релеев. Копии расходились между собой и с
// LxBox, а неизвестный ключ исчезал молча — ровно те болезни, ради которых
// заведён конвейер.
//
// Теперь тело получается ТОЛЬКО так:
//
//	карта sing-box → nodeflow.Sanitize (правила реестра) → nodeflow.Emit
//
// Ни одного `if scheme == …` здесь нет и быть не может: все схемные различия
// живут в contract/registry (SPEC 131 §9 критерий 1).
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// materializeBody — карта outbound'а схемы scheme → каноническое тело узла.
//
// Возвращает тело, коды санитайзера и, если узел собрать нельзя, отказ. При
// отказе body пуст: тело, которое ядро отвергнет фаталом, в state попасть не
// может (CANON §8, инвариант конвейера).
//
// `tag` и `detour` в тело не попадают — их владелец модель узла, а не тело
// (SPEC Т2); `type` наоборот ДОПИСЫВАЕТСЯ первым ключом: тело в state несёт
// его всегда (buildCanonicalServer читает схему узла именно оттуда), а в
// body-секциях реестра его нет, потому что при сборке config.json его пишет
// сборка.
// `source` — вход, которым тело приехало (configtypes.NodeSource*); пусто =
// вход не назван. Его читают правила значений, различающие, кто сочинил
// значение: тело в форме ядра лаунчер не переписывает молча, он
// предупреждает (потолок MTU у AmneziaWG, решение владельца 18.09.2026).
func materializeBody(scheme, source string, outbound map[string]interface{}) (json.RawMessage, []configtypes.Warning, *configtypes.Warning) {
	if _, known := registry.MustGet().Body(scheme); !known {
		// Схема вне реестра — правил для неё нет, и выдумывать их конвейер
		// не вправе. Тело едет как есть, минус managed-ключи сборки; так
		// продолжает работать ручной JSON экзотического типа, ради которого
		// вкладка JSON и существует. Кодов у такого узла тоже нет: сказать
		// про его поля нечего.
		return passthroughBody(outbound)
	}
	res := nodeflow.SanitizeFrom(scheme, source, outbound)
	if res.Drop != nil {
		return nil, res.Warnings, res.Drop
	}
	body, err := nodeflow.Emit(scheme, res.Clean)
	if err != nil {
		drop := configtypes.Warning{
			Code:   "parse_error",
			Params: map[string]string{"error": err.Error()},
		}
		return nil, res.Warnings, &drop
	}
	stamped, err := stampBodyType(scheme, body, outbound)
	if err != nil {
		drop := configtypes.Warning{
			Code:   "parse_error",
			Params: map[string]string{"error": err.Error()},
		}
		return nil, res.Warnings, &drop
	}
	return stamped, res.Warnings, nil
}

// passthroughBody — тело схемы, которой реестр не знает: ключи как пришли,
// порядок алфавитный, tag/detour сняты.
//
// Алфавитный порядок, а не «как в карте»: у Go-карты порядка нет вовсе, и
// без сортировки одно и то же тело меняло бы байты от запуска к запуску —
// узел бы «менялся» на каждом fetch.
func passthroughBody(outbound map[string]interface{}) (json.RawMessage, []configtypes.Warning, *configtypes.Warning) {
	if outbound == nil {
		drop := configtypes.Warning{Code: "parse_error", Params: map[string]string{"error": "no outbound data"}}
		return nil, nil, &drop
	}
	typ, _ := outbound["type"].(string)
	if strings.TrimSpace(typ) == "" {
		drop := configtypes.Warning{Code: "field_missing", Path: "type", Params: map[string]string{"field": "type"}}
		return nil, nil, &drop
	}
	cp := make(map[string]interface{}, len(outbound))
	for k, v := range outbound {
		if k == "tag" || k == "detour" {
			continue
		}
		cp[k] = v
	}
	raw, err := json.Marshal(cp)
	if err != nil {
		drop := configtypes.Warning{Code: "parse_error", Params: map[string]string{"error": err.Error()}}
		return nil, nil, &drop
	}
	obj, err := decodeOrderedJSONObject(raw)
	if err != nil {
		drop := configtypes.Warning{Code: "parse_error", Params: map[string]string{"error": err.Error()}}
		return nil, nil, &drop
	}
	obj.setFirst("type", marshalJSONStringRaw(strings.TrimSpace(typ)))
	return obj.encode(), nil, nil
}

// stampBodyType ставит "type" первым ключом тела.
//
// Имя типа берётся из реестра (`singbox_type` схемы), а не из входной карты:
// иначе ручной JSON диктовал бы тип сам, и один и тот же узел, пришедший
// ссылкой и объектом, получил бы разные тела — то самое расхождение, ради
// закрытия которого заведены парные кейсы корпуса (SPEC 131 §9 критерий 2).
// Схема вне реестра (её тело конвейер не трогает) берёт тип из карты.
func stampBodyType(scheme string, body []byte, outbound map[string]interface{}) (json.RawMessage, error) {
	typ := registry.MustGet().SingboxType(scheme)
	if typ == "" {
		typ, _ = outbound["type"].(string)
		typ = strings.TrimSpace(typ)
	}
	if typ == "" {
		return nil, fmt.Errorf("схема %q: тип узла неизвестен реестру", scheme)
	}
	obj, err := decodeOrderedJSONObject(json.RawMessage(body))
	if err != nil {
		return nil, err
	}
	obj.setFirst("type", marshalJSONStringRaw(typ))
	return obj.encode(), nil
}

// StampBodyType — экспортная обёртка stampBodyType для вызывающих вне пакета.
//
// `nodeflow.Emit` отдаёт тело БЕЗ managed-ключа "type": ставит его тот, кто
// записывает тело в узел. Форма обфускации AmneziaWG писала результат Emit
// как есть — узел оставался без типа, выпадал из сборки («body has no
// "type"») и терял саму форму (она показывается по type == wireguard).
func StampBodyType(scheme string, body []byte, outbound map[string]interface{}) (json.RawMessage, error) {
	return stampBodyType(scheme, body, outbound)
}

// materializeParsedNodeBody — тело разобранного узла плюс ПОЛНЫЙ набор его
// кодов: сначала парсерные (что маппер уже снял на входе), затем
// санитайзерные (что сняли правила реестра).
//
// Порядок слоёв нормирован (CANON §6, ловушка Л14) и не сортируется: сверка
// с корпусом идёт по последовательности разбора. Дубли по паре (code, path)
// снимаются — один и тот же код на одном поле от двух слоёв означает, что
// правило просто сработало дважды, а не что случились два разных события.
func materializeParsedNodeBody(node *configtypes.ParsedNode) (json.RawMessage, []configtypes.Warning, *configtypes.Warning) {
	if node == nil {
		drop := configtypes.Warning{Code: "parse_error", Params: map[string]string{"error": "nil node"}}
		return nil, nil, &drop
	}
	body, sanWarns, drop := materializeBody(node.Scheme, node.Source, outboundMapOf(node))
	return body, mergeWarnings(node.Warnings, sanWarns), drop
}

// outboundMapOf — ПОЛНАЯ карта sing-box разобранного узла.
//
// У ParsedNode адрес живёт в двух местах: в карте Outbound (так его кладут
// URI-парсеры схем) и в полях Server/Port (так его кладёт сборщик хопов
// Xray — `xrayBuildJumpFromSocksOutbound` оставляет в карте лишь version и
// учётные данные). Пока тело писал per-scheme эмиттер, он читал поля
// структуры и разницы не было; тупому эмиттеру карта обязана приезжать
// полной, иначе релей теряет адрес и узел отбрасывается как «без server».
//
// Карта Outbound не правится на месте: она принадлежит вызывающему и живёт
// дольше эмиссии (её читают фильтры Направлений и проверки цепочек).
func outboundMapOf(node *configtypes.ParsedNode) map[string]interface{} {
	src := node.Outbound
	// Безадресные схемы (wireguard, tailscale) адреса в корне тела не имеют
	// вовсе: у wireguard он живёт в peers[], у tailscale его нет — ядро само
	// входит в tailnet по auth_key. ParsedNode.Server у них всё равно
	// заполнен (списки и skip-фильтры читают именно его), и дописывание его
	// в карту дало бы узлу два лишних unknown_key на ровном месте.
	if !schemeHasRootAddress(node.Scheme) {
		return src
	}
	_, hasServer := src["server"]
	_, hasPort := src["server_port"]
	if (hasServer || node.Server == "") && (hasPort || node.Port <= 0) {
		return src
	}
	out := make(map[string]interface{}, len(src)+2)
	for k, v := range src {
		out[k] = v
	}
	if !hasServer && node.Server != "" {
		out["server"] = node.Server
	}
	if !hasPort && node.Port > 0 {
		out["server_port"] = node.Port
	}
	return out
}

// schemeHasRootAddress — есть ли у схемы адрес в КОРНЕ тела.
//
// Ответ берётся из реестра, а не из списка в коде: список схем-исключений —
// это ровно тот вид знания, который кампания и выносит из кода в контракт, и
// вторая его копия здесь разъехалась бы с первой на следующей схеме.
func schemeHasRootAddress(scheme string) bool {
	body, ok := registry.MustGet().Body(scheme)
	if !ok {
		return false
	}
	_, hasServer := body.Fields["server"]
	return hasServer
}

// mergeWarnings склеивает слои кодов с дедупом по паре (code, path),
// сохраняя порядок первого появления.
func mergeWarnings(layers ...[]configtypes.Warning) []configtypes.Warning {
	var out []configtypes.Warning
	at := map[string]int{} // (code, path) → индекс в out
	bare := map[string]int{}
	for _, layer := range layers {
		for _, w := range layer {
			if w.Code == "" {
				continue
			}
			key := w.Code + "\x00" + w.Path
			if _, dup := at[key]; dup {
				continue
			}
			// Тот же код БЕЗ пути от слоя выше (парсер ставит коды до того,
			// как узнаёт путь в теле) уступает место записи с путём: это одно
			// и то же событие, описанное подробнее. Без этого один и тот же
			// мусорный узел давал РАЗНЫЕ наборы кодов ссылкой и объектом —
			// ссылочная половина оставалась без путей (SPEC 131 §9 критерий 2).
			if w.Path != "" {
				if i, hasBare := bare[w.Code]; hasBare {
					delete(at, w.Code+"\x00")
					out[i] = w
					at[key] = i
					continue
				}
			}
			at[key] = len(out)
			if w.Path == "" {
				bare[w.Code] = len(out)
			}
			out = append(out, w)
		}
	}
	return out
}

// sanitizeStoredNodeBody — реализация хука state.SanitizeBody: пересчёт
// кодов по УЖЕ СОХРАНЁННОМУ телу узла (SPEC 131 W2c §3.5, ловушка Л3).
//
// Схема берётся из самого тела ("type"), а не из origin: у узла, пришедшего
// ручным JSON или из бэкапа, origin может не быть вовсе, а тип в теле есть
// всегда — им же определяет схему и сборка (buildCanonicalServer).
//
// Тело возвращается ТОЛЬКО когда санитайзер что-то снял или привёл: сравнение
// идёт по байтам после одинаковой нормализации, иначе «изменилось тело»
// показал бы каждый узел на первом же апгрейде.
func sanitizeStoredNodeBody(req state.SanitizeBodyRequest) (*state.SanitizeBodyResult, error) {
	var outbound map[string]interface{}
	if err := json.Unmarshal(req.Body, &outbound); err != nil {
		return nil, fmt.Errorf("тело узла не разбирается: %w", err)
	}
	obType := strings.TrimSpace(mapStringValue(outbound, "type"))
	if obType == "" {
		return nil, fmt.Errorf("в теле узла нет %q", "type")
	}
	scheme, ok := registry.MustGet().NodeSchemeForSingboxType(obType)
	if !ok {
		// Тип, которого реестр узлом не знает, — тело passthrough, правил нет.
		// Узел «посчитан и чист»: сказать про его поля нечего.
		return &state.SanitizeBodyResult{Warnings: []state.NodeWarning{}}, nil
	}

	body, warns, drop := materializeBody(scheme, subscription.NodeSourceFromOriginKind(req.OriginKind), outbound)
	res := &state.SanitizeBodyResult{Warnings: stateWarnings(warns)}
	if res.Warnings == nil {
		res.Warnings = []state.NodeWarning{}
	}
	if drop != nil {
		// Узел не сносим: он уже в состоянии человека, и молчаливый снос на
		// апгрейде хуже, чем узел с кодами. Тело оставляем как есть — пусть
		// с ним разбирается сборка и `sing-box check`.
		res.Drop = true
		return res, nil
	}
	if bodiesEquivalent(req.Body, body) {
		return res, nil // санитайзер ничего не снял — байты не трогаем
	}
	res.Body = body
	return res, nil
}

// bodiesEquivalent — совпадают ли тела с точностью до порядка ключей и
// формы пробелов.
//
// Сравнивать сырые байты нельзя: сохранённое тело писал прежний эмиттер, и
// порядок ключей у него свой. Различие в ПОРЯДКЕ — не деградация, и
// перезаписывать ради него файл человека не за что.
func bodiesEquivalent(a, b []byte) bool {
	var av, bv interface{}
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	aj, aerr := json.Marshal(av)
	bj, berr := json.Marshal(bv)
	if aerr != nil || berr != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}

// dropReason — английский текст отказа для записи `unsupported`/`dropped`.
//
// `Reason` и `Warnings` — разные поля с разными адресатами (ловушка Л20):
// первое едет в диагностику как есть, второе переводится в UI по коду.
// Здесь берётся текст кода из реестра; без записи в реестре остаётся сам код
// — он всё равно опознаваем, в отличие от пустой строки.
func dropReason(w *configtypes.Warning) string {
	if w == nil {
		return ""
	}
	_, text, ok := registry.MustGet().WarningText(w.Code, "en", w.Params)
	if ok && strings.TrimSpace(text) != "" {
		return text
	}
	return w.Code
}

// mapStringValue — строковое поле карты или "".
//
// Переехало из outbound_tls_emit.go вместе со сносом старого эмиттера
// (контракт 1.1.11): от того файла это единственное, что пережило удаление, —
// остальное было копиями правил реестра.
func mapStringValue(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}
