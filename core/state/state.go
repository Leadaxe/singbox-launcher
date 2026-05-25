// Package state — модель декларативного состояния Configurator (бывшего
// Wizard) без UI-зависимостей.
//
// SPEC 052 (CONNECTIONS_REDESIGN): на диск пишется v5/v6-схема. Поверхностный
// in-memory тип State сохраняет legacy-форму (ParserConfig.ParserConfig.Proxies,
// Vars, CustomRules, DNSOptions) для совместимости с существующими
// callsite'ами; canonical секция Connections живёт параллельно и
// синхронизируется на Save (UI-edits ParserConfig → Sync → write).
//
// SPEC 060: v5/ и v6/ subpackages collapsed в единый core/state/. Wire format
// не меняется. Историческое имя поля RulesV6 сохранено в Phase 2/3/4 и
// переименовано в Rules на Phase 5.
//
// Этот пакет НЕ:
//   - не зависит от UI / Fyne;
//   - не делает ParseAndPreview / fetch подписок (это слой parser);
//   - не пишет config.json (это слой build);
//   - не реактивен сам по себе.
package state

import (
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SchemaVersion — версия on-disk-формата state.json, которую пишет Save.
//
// История:
//   - v2 — самый ранний формат;
//   - v3 — rules library: единый custom_rules + rules_library_merged;
//   - v4 — SPEC 032 (vars + literals + if/if_or в params);
//   - v5 — SPEC 052: top-level meta + connections, per-source meta/raw cache.
//   - v6 — SPEC 053/056: rules[] kind discriminator, dns_options flat shape.
//
// SPEC 060 Phase 5: SchemaVersion теперь всегда v6 — dual write path удалён.
// Load принимает v2/v3/v4/v5 (с авто-миграцией); Save всегда пишет v6.
const SchemaVersion = SchemaVersionV6

// ── State ────────────────────────────────────────────────────────

// State — корневая декларативная модель.
//
// Изменения этого типа должны быть JSON-обратно совместимы с v5/v6.
// Top-level legacy-поля (ID, RulesLibraryMerged, SelectableRuleStates) не
// сериализуются в v5 — оставлены в памяти только для backward-compat
// callsite'ов и одноразовой миграции с v3/v4.
type State struct {
	// === Identity / Meta ===

	// Version — версия формата файла, прочитанная при Load (или текущая
	// SchemaVersion при создании в памяти). Save всегда пишет SchemaVersion.
	Version int

	// ID — legacy (v2-v4). Snapshot-имя теперь живёт в имени файла
	// bin/wizard_states/<name>.json. Не сериализуется в v5; сохраняется
	// в памяти для callsite'ов которые ещё его читают (state_store, dialogs).
	ID string

	// Comment — пользовательский комментарий, сериализуется в meta.comment.
	Comment string

	// CreatedAt / UpdatedAt — время создания / последней записи.
	// Сериализуются как RFC3339-строки в meta.{created_at,updated_at}.
	CreatedAt time.Time
	UpdatedAt time.Time

	// === Legacy proxies-view (UI / dashboard / parser callsite'ы) ===

	// ParserConfig — proxies (sources) + global outbounds в legacy-форме
	// (configtypes.ParserConfig.ParserConfig.{Proxies,Outbounds,Parser}).
	//
	// SPEC 052: эта view ДЕРИВНАЯ от Connections. На Load v5 заполняется
	// reverse-адаптером из Connections.Sources; на Save сначала
	// SyncConnectionsFromLegacy, затем write v5.
	ParserConfig configtypes.ParserConfig

	// === Connections (v5 canonical) ===

	// Connections — sources + global outbounds + defaults в v5-форме.
	// Источник истины для нового кода (parser adapter / Rebuild / UI после
	// Phase 7).
	Connections ConnectionsSection

	// === Common (template / rules) ===

	// ConfigParams — параметры маршрутизации (route.final и т.п.).
	ConfigParams []ConfigParam

	// Vars — переопределения переменных шаблона (vars из вкладки Settings).
	Vars []SettingVar

	// SelectableRuleStates — снимок выбора пользователя для template-rules.
	// Legacy (v2-v4); в v5 не сериализуется (rules library полностью в
	// CustomRules после SPEC 027). Поле остаётся для одноразовой миграции
	// и для UI-кода, который ещё на него ссылается (rules_library.go).
	SelectableRuleStates []SelectableRuleState

	// CustomRules — пользовательские правила.
	CustomRules []CustomRule

	// RulesLibraryMerged — флаг SPEC 027: rules library уже мигрирована.
	// Legacy; в v5 не сериализуется (всегда true). В памяти сохраняется
	// чтобы UI-код не ре-запускал миграцию каждый Load.
	RulesLibraryMerged bool

	// DNSOptions — снимок вкладки DNS визарда (v5 legacy shape).
	// Приватный тип LegacyDNSOptionsV5 — оставлен для backward-compat с UI
	// кодом который ещё работает через legacy view. В v6 path обычно nil.
	DNSOptions *LegacyDNSOptionsV5

	// === SPEC 053: v6 preset bundles ===

	// Rules — новая модель правил (kind discriminator: preset/inline/srs).
	// SPEC 053: thin-ref preset bundles. Заполняется при load v6 файлов;
	// при load v5 — derived из CustomRules через migrateV5ToV6.
	//
	// SPEC 060 Phase 5: rename RulesV6 → Rules. JSON tag всё ещё "rules".
	Rules []Rule

	// DNS — новая DNS-секция (SPEC 056-R-N: flat kind discriminator
	// template/preset/user для servers и preset/user для rules).
	// Параллельно DNSOptions (legacy v5) для одностороннего sync на Save.
	// JSON-ключ на диске: "dns_options" (см. state.marshalDisk).
	// Историческое имя поля было DNSV6 (когда v5/v6 co-existed); после
	// SPEC 056-R-N оба формата это v6 internally, суффикс выкинут.
	DNS DNSOptions
}

// SelectableRuleState — выбор пользователя для правила, определённого в шаблоне.
// Legacy v2-v4; в v5 не сериализуется (см. SPEC 027).
type SelectableRuleState struct {
	Label            string `json:"label"`
	Enabled          bool   `json:"enabled"`
	SelectedOutbound string `json:"selected_outbound"`
}

// New создаёт новый State с актуальной SchemaVersion и текущим UTC-временем.
func New() *State {
	now := time.Now().UTC()
	return &State{
		Version:      SchemaVersion,
		CreatedAt:    now,
		UpdatedAt:    now,
		ConfigParams: []ConfigParam{},
		CustomRules:  []CustomRule{},
	}
}

// GetSubscriptionSources возвращает только source'ы типа subscription
// из Connections.Sources (для parser adapter и UI).
func (s *State) GetSubscriptionSources() []Source {
	if s == nil {
		return nil
	}
	out := make([]Source, 0, len(s.Connections.Sources))
	for _, src := range s.Connections.Sources {
		if src.Type == SourceTypeSubscription {
			out = append(out, src)
		}
	}
	return out
}

// GetServerSources возвращает только source'ы типа server.
func (s *State) GetServerSources() []Source {
	if s == nil {
		return nil
	}
	out := make([]Source, 0, len(s.Connections.Sources))
	for _, src := range s.Connections.Sources {
		if src.Type == SourceTypeServer {
			out = append(out, src)
		}
	}
	return out
}

// FindSource ищет Source по ID. Возвращает nil если не найден.
func (s *State) FindSource(id string) *Source {
	if s == nil {
		return nil
	}
	for i := range s.Connections.Sources {
		if s.Connections.Sources[i].ID == id {
			return &s.Connections.Sources[i]
		}
	}
	return nil
}
