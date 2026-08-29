// File migrate_materialize.go — материализация узлов для миграции v6 → v7
// (SPEC 118 W2, PLAN §5 шаг 1).
//
// Реализация хуков state.MigrationHooks: единственный легальный потребитель
// старого raw-кэша вне fetch-пути. Тело подписки гонится через НОВЫЙ чистый
// парсер (subscription.ParseSubscriptionBody — skip/дедуп/уникализация/кап),
// поэтому body мигрированных узлов и body свежего fetch W3 — один код:
// иначе первый fetch после апгрейда дал бы массовый «body изменился».
//
// Живёт в пакете config, а не state: эмиттеры outbound-JSON и парсер
// подписок сами импортируют state — прямой вызов из state дал бы цикл
// импорта (тот же приём, что subscription.NodeIdentityFunc, node_hash.go).
package config

import (
	"encoding/json"
	"fmt"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// init подставляет реализацию в state: пакет config импортируется каждым
// путём, который зовёт state.Load (core, UI, debugapi), поэтому к моменту
// первой миграции хуки уже стоят. Тесты пакета state в изоляции работают с
// nil-хуком — миграция честно предупреждает вместо материализации.
func init() {
	state.SetMigrationHooks(state.MigrationHooks{
		MaterializeSubscription: materializeSubscriptionForMigration,
		MaterializeServer:       materializeServerForMigration,
	})
}

// materializeSubscriptionForMigration — raw-тело подписки → канонические
// узлы v7 + финальные теги старой тег-машины + legacy-хэши.
func materializeSubscriptionForMigration(req state.MigrationSubRequest) (*state.MigrationSubResult, error) {
	body := req.Body
	// Raw-файл хранит недекодированное тело (как пришло по сети) — fetch
	// отдаёт парсеру уже декодированное; мимикрируем тот же контракт
	// (см. core/rebuild_raw_cache.go buildBodyLookup).
	if dec, err := subscription.DecodeSubscriptionContent(body); err == nil {
		body = dec
	}

	pb, err := subscription.ParseSubscriptionBody(body, req.Skip, req.MaxNodes)
	if pb == nil {
		return nil, err
	}
	res := &state.MigrationSubResult{
		Truncated: pb.Truncated,
		Warnings:  append([]string(nil), pb.Warnings...),
	}
	if err != nil {
		res.Warnings = append(res.Warnings, err.Error())
	}

	for _, e := range pb.Entries {
		if e == nil || e.Node == nil {
			continue
		}
		// Финальный тег считается ДО правок узла и без записи в node.Tag:
		// {$tag} в политике читает сырой провайдерский тег — ровно как в
		// старом applyURINodeTags/applyTagsToSingboxNode.
		finalTag := subscription.ApplyLegacyTagMachine(e.Node, req.TagPrefix, req.TagPostfix, req.TagMask, e.Num, req.TagCounts)

		node, convErr := canonicalNodeFromEntry(req.SubID, e)
		if convErr != nil {
			// Битая запись — деградация записи, не подписки (SPEC Т3):
			// старый движок такой узел тоже не доносил до конфига.
			res.Warnings = append(res.Warnings, fmt.Sprintf("node %q not emittable — dropped: %v", e.RawTag, convErr))
			continue
		}
		mat := state.MigrationMaterializedNode{Node: node, FinalTag: finalTag}
		if node.Kind == state.SourceKindServer {
			mat.LegacyHash = LegacyNodeIdentityHash(e.Node)
		}
		res.Nodes = append(res.Nodes, mat)
	}
	return res, nil
}

// canonicalNodeFromEntry — общая конверсия принятой записи тела в
// канонический узел v7 (server с body/origin либо auto с group).
//
// Один код на миграцию W2 и fetch W3 намеренно: body мигрированных узлов и
// body свежего fetch обязаны совпадать байт-в-байт — иначе первый fetch
// после апгрейда дал бы массовый «body изменился».
func canonicalNodeFromEntry(subID string, e *subscription.ParsedBodyEntry) (state.Node, error) {
	var origin *state.Origin
	if e.OriginKind != "" {
		origin = &state.Origin{Kind: e.OriginKind, Raw: e.OriginRaw}
	}

	if e.Node.Scheme == configtypes.SchemeGroup {
		group := &state.AutoGroup{
			GroupType: e.GroupType,
			Default:   e.GroupDefaultRaw,
			Members:   make([]state.NodeLink, 0, len(e.MemberRawTags)),
			Strategy:  autoStrategyFromGroupOptions(e.Node.Outbound),
		}
		for _, raw := range e.MemberRawTags {
			// Члены резолвятся на узлы ТОЙ ЖЕ подписки по сырым тегам —
			// контейнер и есть связь (features/sources.md «Auto»).
			group.Members = append(group.Members, state.NodeLink{FolderID: subID, Tag: raw})
		}
		// default только у selector; urltest со stray default не плодим.
		if group.GroupType != state.AutoGroupSelector {
			group.Default = ""
		}
		return state.Node{
			Kind:    state.SourceKindAuto,
			Tag:     e.RawTag,
			Enabled: true,
			Origin:  origin,
			Group:   group,
		}, nil
	}

	bodyJSON, emitErr := emitMigrationBody(e.Node)
	if emitErr != nil {
		return state.Node{}, emitErr
	}
	return state.Node{
		Kind:    state.SourceKindServer,
		Tag:     e.RawTag,
		Enabled: true,
		Origin:  origin,
		Body:    bodyJSON,
	}, nil
}

// materializeServerForMigration — body корневого server-источника из URI
// либо ручного config_json (приоритет за config_json — как у старого
// парсера: раз пользователь сохранил ручной JSON, URI намеренно
// игнорируется).
func materializeServerForMigration(req state.MigrationServerRequest) (*state.MigrationServerResult, error) {
	if len(req.ConfigJSON) > 0 {
		node, err := subscription.NodeFromManualConfigJSON(req.ConfigJSON)
		if err != nil {
			return nil, fmt.Errorf("manual config_json: %w", err)
		}
		body, err := stripTagAndDetour(req.ConfigJSON)
		if err != nil {
			return nil, fmt.Errorf("manual config_json: %w", err)
		}
		return &state.MigrationServerResult{
			Body:       body,
			OriginKind: state.OriginKindJSON,
			OriginRaw:  string(req.ConfigJSON),
			LegacyHash: LegacyNodeIdentityHash(node),
		}, nil
	}

	line := subscription.NormalizeSubscriptionTextLine(req.URI)
	if line == "" {
		return nil, fmt.Errorf("no URI and no config_json")
	}
	node, err := subscription.ParseNode(line, nil)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("URI parsed to no node")
	}
	body, err := emitMigrationBody(node)
	if err != nil {
		return nil, err
	}
	return &state.MigrationServerResult{
		Body:       body,
		OriginKind: state.OriginKindURI,
		OriginRaw:  req.URI, // байт в байт, как хранился
		LegacyHash: LegacyNodeIdentityHash(node),
	}, nil
}

// emitMigrationBody — канонический body узла: эмиссия существующим
// эмиттером (wireguard → endpoint-эмиттер, SPEC 101) и зачистка tag/detour —
// body чист от detour (SPEC Т2), тег живёт в Node.Tag.
func emitMigrationBody(node *configtypes.ParsedNode) (json.RawMessage, error) {
	var emitted string
	var err error
	if node.Scheme == "wireguard" {
		emitted, err = GenerateEndpointJSONBare(node)
	} else {
		emitted, err = GenerateNodeJSONBare(node)
	}
	if err != nil {
		return nil, err
	}
	return stripTagAndDetour(json.RawMessage(emitted))
}

// stripTagAndDetour убирает из outbound-объекта ключи tag и detour.
// Пересборка через map сортирует ключи — стабильно между запусками; W4
// всё равно разбирает body в map перед эмиссией.
func stripTagAndDetour(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("body is not a JSON object: %w", err)
	}
	delete(obj, "tag")
	delete(obj, "detour")
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// autoStrategyFromGroupOptions — allowlist-перенос опций провайдерской
// группы в AutoStrategy (PLAN §3.1): url / interval / idle_timeout /
// tolerance / interrupt_exist_connections. Неизвестные и невыразимые ключи
// отбрасываются — форма настроек та же, что у Направления, и лишнему в ней
// места нет (тот же принцип, что state.autoFromLegacyGroup).
func autoStrategyFromGroupOptions(outbound map[string]interface{}) configtypes.DirectionAuto {
	var auto configtypes.DirectionAuto
	if outbound == nil {
		return auto
	}
	if s, ok := outbound["url"].(string); ok {
		auto.URL = s
	}
	if s, ok := outbound["interval"].(string); ok {
		auto.Interval = s
	}
	if s, ok := outbound["idle_timeout"].(string); ok {
		auto.IdleTimeout = s
	}
	switch t := outbound["tolerance"].(type) {
	case float64:
		auto.Tolerance = configtypes.NewTemplateInt(int(t))
	case int:
		auto.Tolerance = configtypes.NewTemplateInt(t)
	}
	if b, ok := outbound["interrupt_exist_connections"].(bool); ok {
		flag := b
		auto.InterruptExistConnections = &flag
	}
	return auto
}
