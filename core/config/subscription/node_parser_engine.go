// Package subscription: разбор ссылки ДВИЖКОМ реестра (SPEC 133).
//
// Единственный путь разбора. Какая схема ведёт ссылку, решает не код, а
// РЕЕСТР — своим `detect`; имён схем в этом файле нет по той же причине, по
// какой их нет в самом движке.
//
// Кампания закончена: рукописных парсеров ссылок не осталось, вместе с ними
// ушли временный атрибут секции `live` и страж остатка. Текст, который не
// опознала ни одна секция, — «схема не поддержана», а не повод искать второй
// путь.
package subscription

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/linkmap"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/textnorm"
)

// parseURIByEngine разбирает ссылку секцией реестра.
//
// Третье возвращаемое — «секции на этот текст нет». Это НЕ ошибка разбора:
// такую ссылку не ведёт никто, и вызывающий отвечает «схема не поддержана».
// Отдельным значением, а не ошибкой, потому что различать их обязан он: у
// неразобравшейся ссылки причина едет человеку, у неопознанной — нет.
func parseURIByEngine(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error, bool) {
	plans, err := linkmap.Planes()
	if err != nil {
		// Реестр не собрался — это отказ сборки, а не свойство ссылки.
		// Старый путь на сломанном реестре тоже не спасёт, но и ронять им
		// разбор всей подписки незачем: пусть ведёт он.
		return nil, nil, false
	}
	scheme, plan, ok := linkmap.SelectURI(plans, uri)
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
		//
		// Берётся из РАСПАКОВАННОГО пространства, а не повторным разбором
		// исходного текста: у обёрнутых форм (base64-пейлоад hysteria2,
		// контейнер vmess) снаружи нет ни одного параметра, и справка
		// выходила бы пустой там, где ссылка их несёт.
		Query: res.Query,
	}
	applyEngineBody(node, plan, res.Body)
	node.UUID = credentialFromBody(plan, res)

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
//
// Где именно лежит адрес, знает СЕКЦИЯ: у обычного outbound'а это корневые
// `server`/`server_port`, а у endpoint'а wireguard — `peers[].address` и
// `peers[].port`, и секция объявляет их в label.fallback.server_path/port_path
// (тем же местом, откуда шаблон дефолтного тега берёт адрес). Списка схем
// здесь нет по той же причине, что и в движке.
func applyEngineBody(node *configtypes.ParsedNode, plan *linkmap.Plan, body map[string]interface{}) {
	serverPath, portPath := "server", "server_port"
	if plan != nil && plan.Mapper != nil && plan.Mapper.Label != nil && plan.Mapper.Label.Fallback != nil {
		if p := plan.Mapper.Label.Fallback.ServerPath; p != "" {
			serverPath = p
		}
		if p := plan.Mapper.Label.Fallback.PortPath; p != "" {
			portPath = p
		}
	}
	if s, ok := linkmap.BodyString(body, serverPath); ok {
		node.Server = s
	}
	if p, ok := linkmap.BodyInt(body, portPath); ok {
		node.Port = p
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
func credentialFromBody(plan *linkmap.Plan, res *linkmap.Result) string {
	ui := plan.Mapper.UserInfo
	if ui == nil || len(ui.Into) == 0 {
		return ""
	}
	body := res.Body
	// `uuid` в теле — учётные данные САМ ПО СЕБЕ, где бы он ни лежал.
	//
	// Правило «первый компонент userinfo» описывает не все схемы: у vmess
	// userinfo это `method:uuid` (как у ss — `method:password`), то есть
	// первый компонент — ШИФР, а не идентификатор; у формы-контейнера
	// v2rayN userinfo нет вовсе, и uuid приезжает ключом объекта. Прежний
	// путь обе формы сводил к одному (`node.UUID = id`,
	// node_parser_vmess.go:228 и :153), и то же делают оба обратных
	// перевода — canonicalCredential и singboxCredentialFromMap, у которых
	// vmess стоит в ветке `uuid`. Так что вопрос решает ТЕЛО, а не позиция
	// в userinfo.
	if s, ok := body["uuid"].(string); ok && s != "" {
		return s
	}
	// Форма БЕЗ userinfo и без uuid — поля нет: позиция в `into` относится
	// к userinfo, которого у этой формы не было.
	if !res.HadUserInfo {
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
