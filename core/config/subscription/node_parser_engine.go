// Package subscription: вход разбора ссылки ЧЕРЕЗ ДВИЖОК реестра (SPEC 133).
//
// Развилка «движок или рукописный парсер» одна и живёт здесь. Решает её не
// код, а РЕЕСТР: секция схемы помечена `live: true`, и её собственный `detect`
// опознаёт текст ссылки. Имён схем в этом файле нет — по той же причине, по
// какой их нет в самом движке: появись здесь список, кампания кончилась бы
// переносом switch'а из одного файла в другой.
//
// Развилка ВРЕМЕННАЯ. Когда `live` стоит у всех секций, этот файл и старый
// вход исчезают вместе с атрибутом — движок остаётся единственным путём.
// Остаток держит страж TestMappersWithoutLive.
package subscription

import (
	"fmt"
	"net/url"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/linkmap"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/textnorm"
)

// parseURIByEngine разбирает ссылку живой секцией реестра.
//
// Третье возвращаемое — «движок эту ссылку не ведёт»: живой секции, чей detect
// её опознаёт, нет, и вызывающий обязан пойти старым путём. Это НЕ ошибка
// разбора: до конца кампании таких ссылок большинство.
func parseURIByEngine(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error, bool) {
	plans, err := linkmap.Planes()
	if err != nil {
		// Реестр не собрался — это отказ сборки, а не свойство ссылки.
		// Старый путь на сломанном реестре тоже не спасёт, но и ронять им
		// разбор всей подписки незачем: пусть ведёт он.
		return nil, nil, false
	}
	scheme, plan, ok := linkmap.SelectLiveURI(plans, uri)
	if !ok {
		return nil, nil, false
	}

	reg, regErr := registry.Get()
	if regErr != nil {
		return nil, nil, false
	}
	bodyType := reg.SingboxType(scheme)

	res, execErr := linkmap.ParseURI(plan, uri, bodyType, nil)
	if execErr != nil {
		return nil, fmt.Errorf("invalid %s URI: %w", scheme, execErr), true
	}

	node := &configtypes.ParsedNode{
		Scheme: schemeOfNode(plan, uri, scheme),
		Source: nodeflow.SourceURI,
		// Query остаётся ЧИТАЕМЫМ для фильтров и совместимости: это не вход
		// разбора (движок читает своё пространство), а справка о ссылке, по
		// которой работают skip-фильтры и вызывающие за пределами разбора.
		Query: queryOfURI(uri),
	}
	applyEngineBody(node, res.Body)
	node.UUID = credentialFromBody(plan, res.Body)

	node.Label = textnorm.NormalizeProxyDisplay(sanitizeForDisplay(res.Label))
	node.Tag, node.Comment = extractTagAndComment(node.Label)
	if node.Tag == "" {
		// Фолбэк тега — `{scheme}-{server}-{server_port}`, и ИМЕННО ТО
		// написание схемы, которое объявила секция своим
		// label.fallback.scheme_source: node.Scheme его уже несёт. Подставить
		// сюда имя схемы реестра значило бы переименовать socks5-host-1080 в
		// socks-host-1080 у всех живых узлов — тег входит в identity, и за
		// ним тянутся отметки disabled и ссылки цепочек (D133-6, пока НЕ
		// принято к исполнению).
		node.Tag = generateDefaultTag(node.Scheme, node.Server, node.Port)
		node.Comment = node.Tag
	}
	node.Tag = normalizeFlagTag(node.Tag)
	node.Flow = node.Query.Get("flow")

	// Коды движка становятся деградациями узла. Формат warning'а принадлежит
	// подписке, а не движку, поэтому перекладывает их вызывающий (Result.Note).
	for _, n := range res.Notes {
		if len(n.Params) > 0 {
			node.AddWarningWithParams(n.Code, n.Params)
			continue
		}
		node.AddWarning(n.Code)
	}

	if shouldSkipNode(node, skipFilters) {
		return nil, nil, true
	}

	// Тело собрал движок целиком: buildOutbound здесь не зовётся — он и есть
	// тот рукописный путь, который кампания заменяет.
	node.Outbound = res.Body
	node.Outbound["type"] = bodyType
	node.Outbound["tag"] = node.Tag
	return node, nil, true
}

// schemeOfNode — что попадает в ParsedNode.Scheme: имя схемы реестра или
// НАПИСАНИЕ из ссылки.
//
// Решает это сама секция своим `label.fallback.scheme_source`, потому что от
// того же выбора зависит дефолтный тег (`{scheme}-{server}-{server_port}`), а
// тег входит в identity узла. У socks написание сохраняется намеренно:
// канонизация socks5 → socks переименовала бы тег socks5-host-1080 у ВСЕХ
// живых узлов и сбросила бы отметки disabled и ссылки цепочек
// (node_parser_core.go:316-325, docs/IDENTITY.md §4a-C).
//
// Два источника истины здесь не заводятся: атрибут один, и Scheme с тегом
// расходиться не могут по построению.
func schemeOfNode(plan *linkmap.Plan, uri, scheme string) string {
	if plan == nil || plan.Mapper == nil || plan.Mapper.Label == nil ||
		plan.Mapper.Label.Fallback == nil ||
		plan.Mapper.Label.Fallback.SchemeSource != "as_written" {
		return scheme
	}
	if i := strings.Index(uri, "://"); i > 0 {
		return strings.ToLower(uri[:i])
	}
	return scheme
}

// applyEngineBody снимает с тела поля, которые ParsedNode держит отдельно.
//
// Сервер и порт дублируются: тело едет в конфиг ядра, а поля структуры читают
// дедуп, фильтры и UI. Разъехаться они не могут — источник один.
func applyEngineBody(node *configtypes.ParsedNode, body map[string]interface{}) {
	if s, ok := body["server"].(string); ok {
		node.Server = s
	}
	switch p := body["server_port"].(type) {
	case int:
		node.Port = p
	case float64:
		node.Port = int(p)
	}
}

// credentialFromBody — что кладётся в ParsedNode.UUID.
//
// Поле историческое и плохо названное: у vless/vmess/tuic там UUID, у
// trojan/ss/anytls — пароль, у ssh и naive — имя пользователя. Общее у них
// одно: это ПЕРВЫЙ компонент userinfo, и прежний путь так его и брал —
// `node.UUID = parsedURL.User.Username()` (node_parser_core.go:465).
//
// Отсюда и правило: поле, куда секция направила userinfo.into[0]. Отметка
// `secret` для этого не годится — у naive секрет это password (ВТОРОЙ
// компонент), а в UUID прежний путь клал username, и корпус на этом стоит.
//
// Поле выводимое, а не самостоятельное: обратный путь восстанавливает его
// из готового тела тем же способом (canonicalCredential, canonical_emit.go).
func credentialFromBody(plan *linkmap.Plan, body map[string]interface{}) string {
	ui := plan.Mapper.UserInfo
	if ui == nil || len(ui.Into) == 0 {
		return ""
	}
	// Одиночный userinfo, уехавший по single_into, первым компонентом не
	// является: у naive `secret@host` это ПАРОЛЬ, и в UUID он не попадает
	// (корпус password_only_userinfo, QUIRKS Q133-43).
	if s, ok := body[ui.Into[0]].(string); ok {
		return s
	}
	return ""
}

// queryOfURI достаёт query ссылки для фильтров и вызывающих.
//
// Отказ url.Parse здесь НЕ отказ разбора: движок уже разобрал ссылку своим
// лексером, который принимает то, что net/url отвергает (multi-port
// authority). Пустой набор означает «справки нет», а не «узел битый».
func queryOfURI(uri string) url.Values {
	i := strings.Index(uri, "?")
	if i < 0 {
		return url.Values{}
	}
	rest := uri[i+1:]
	if h := strings.Index(rest, "#"); h >= 0 {
		rest = rest[:h]
	}
	q, err := url.ParseQuery(rest)
	if err != nil {
		return url.Values{}
	}
	return q
}
