package build

import (
	"encoding/json"

	"singbox-launcher/core/state"
)

// ParsedCache — in-memory результат парсинга подписок: готовые к вставке
// JSON-блоки sing-box outbound + WireGuard endpoint объекты.
//
// SPEC 052 phase 8: формат `bin/outbounds.cache.json` удалён;
// `core/outboundscache` package retired. Эта структура — pure-data carrier
// между Update/Rebuild (в `core/`) и BuildConfig (в `core/build/`).
//
// Заполняется одним из:
//   - `core.refreshSubscriptionsMetaAndCache` после успешного fetch'а
//     (Update path); парсер формирует `[]string`-блоки → `jsonStringsToRawMessages`
//     → этот тип.
//   - `core.buildSnapshotFromRawCache` при Rebuild без сети (читает
//     `bin/subscriptions/*.raw`).
//   - In-memory mode визарда: `business.inMemoryCacheFromModel` для preview.
type ParsedCache struct {
	// Outbounds — готовые JSON-блоки sing-box outbound (для @ParserSTART/@ParserEND).
	Outbounds []json.RawMessage

	// Endpoints — WireGuard endpoints (если используются).
	Endpoints []json.RawMessage

	// Warnings — non-fatal замечания парсера, о которых должен узнать
	// пользователь (например, деградированные naive-ноды на ядре без
	// with_naive_outbound). Caller (RebuildConfigIfDirty) присоединяет их
	// к Result.Validation.Warnings; BuildConfig это поле не читает.
	Warnings []string

	// NodeOrigins — финальный тег узла → источник, из которого он приехал
	// (SPEC 113-B). Граф-санитайзер видит только теги; выбросив узел за
	// висячий detour, он обязан назвать источник, у которого сломался
	// переход, — иначе исключение снова становится молчаливым. Пустая карта
	// не ошибка: узел тогда назовут собственным тегом.
	NodeOrigins map[string]NodeOrigin

	// NodeSections — секции узлов, дошедших до эмиссии (SPEC 121), в порядке
	// эмиссии. Узел, не попавший в конфиг (выключен, не собрался, снят
	// граф-санитайзером), сюда не входит: секции живут и умирают вместе с
	// узлом, и фрагмент без своего узла ссылался бы в никуда.
	NodeSections []NodeSectionSet
}

// NodeSectionSet — секции ОДНОГО узла плюс всё, чем сборка их адресует
// (SPEC 121 §10.2).
//
// Записи здесь в форме ХРАНЕНИЯ, до подстановки `@self`: финальный тег узла
// известен только здесь, и подставляет его инъекция в preset_merge.go.
type NodeSectionSet struct {
	// FinalTag — тег, под которым узел эмитится в конфиг (после тег-политики
	// контейнера). Значение плейсхолдера `@self` / `@{self}`.
	FinalTag string
	// Link — идентичность узла в состоянии ({FolderID, сырой тег}).
	Link NodeLink
	// Sections — записи узла: route-правила (inline|srs) и DNS-записи (user).
	Sections *state.NodeSections
}

// RulesWithSelf — route-правила узла с подставленным финальным тегом, готовые
// к конкатенации со списком state.Rules.
//
// Подстановка идёт по ТЕЛУ записи (json.RawMessage), а не по разобранной
// карте: порядок ключей match-объекта значим ровно так же, как у тела узла.
func (s NodeSectionSet) RulesWithSelf() []state.Rule {
	if s.Sections == nil || len(s.Sections.Rules) == 0 {
		return nil
	}
	out := make([]state.Rule, 0, len(s.Sections.Rules))
	for _, r := range s.Sections.Rules {
		cp := r
		if r.Num != nil {
			// Копия номера: список правил дальше сортируется и раздаётся,
			// а состояние правку номера здесь не заказывало.
			n := *r.Num
			cp.Num = &n
		}
		if len(r.Refs) > 0 {
			cp.Refs = append([]string(nil), r.Refs...)
		}
		if len(r.Vars) > 0 {
			vars := make(map[string]string, len(r.Vars))
			for k, v := range r.Vars {
				vars[k] = v
			}
			cp.Vars = vars
		}
		cp.Body = state.SubstituteSelf(r.Body, s.FinalTag)
		out = append(out, cp)
	}
	return out
}

// DNSServersWithSelf / DNSRulesWithSelf — DNS-записи узла с подставленным
// финальным тегом.
func (s NodeSectionSet) DNSServersWithSelf() []state.DNSServer {
	servers := s.Sections.DNSServers()
	if len(servers) == 0 {
		return nil
	}
	out := make([]state.DNSServer, 0, len(servers))
	for _, srv := range servers {
		cp := srv
		cp.Tag = state.SubstituteSelfInString(srv.Tag, s.FinalTag)
		cp.Body = state.SubstituteSelfInMap(srv.Body, s.FinalTag)
		out = append(out, cp)
	}
	return out
}

func (s NodeSectionSet) DNSRulesWithSelf() []state.DNSRule {
	rules := s.Sections.DNSRules()
	if len(rules) == 0 {
		return nil
	}
	out := make([]state.DNSRule, 0, len(rules))
	for _, r := range rules {
		cp := r
		cp.Body = state.SubstituteSelfInMap(r.Body, s.FinalTag)
		out = append(out, cp)
	}
	return out
}

// NodeLink — ссылка на узел в форме сборки (зеркало state.NodeLink и
// configtypes.NodeLink; core/build — leaf-пакет и о них не знает).
type NodeLink struct {
	// FolderID — ULID контейнера; "" = корневое пространство тегов.
	FolderID string
	// Tag — сырой тег узла в его контейнере.
	Tag string
}

// NodeOrigin — чей это узел: ULID источника и его человеческая подпись.
// Зеркалит config.NodeOrigin; своё определение здесь, потому что core/build
// про core/config не знает (и не должен: зависимость идёт в другую сторону).
type NodeOrigin struct {
	SourceID    string
	SourceLabel string
}

// SourceExclusion — источник, чьи узлы выброшены на последнем рубеже
// (SPEC 113-B). Та же тройка, что у config.SourceExclusion, плюс счётчик;
// вызывающий доливает эти записи в отчёт сборки поверх записей парсера.
//
// DroppedNodes (SPEC 115) — СКОЛЬКО узлов источника снято. Санитайзер работает
// по узлам, а отчёт группируется по источнику: у подписки на 500 узлов один
// сломанный переход снимает их все разом, и без числа пометка «источник
// пострадал» не отличает потерю одного узла от потери всей подписки.
type SourceExclusion struct {
	SourceID     string
	SourceLabel  string
	Reason       string
	DroppedNodes int
	// MissingTarget — тег цели detour, которой не оказалось в собранном
	// конфиге; пусто, когда узлы сняты по другой причине (кольцо ссылок).
	// Отдельным полем, а не разбором Reason: отчёт заводит на такую потерю
	// собственный вид записи (target_missing, SPEC 115 §2), и вытаскивать тег
	// обратно из человеческого текста было бы разбором собственного вывода.
	MissingTarget string
}
