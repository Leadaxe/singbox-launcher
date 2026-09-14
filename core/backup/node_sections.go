// File node_sections.go — секции узла на входе бэкапа (SPEC 121 §10.5,
// SPEC 126 L2/L3, SPEC 127 §6.2 W2.5).
//
// Форма записей секции — та же, что у корневых `rules[]`/`dns` (NODE_SECTIONS.md
// §1): «второго парсера под внутреннюю форму» быть не должно. В состоянии v8
// это `state.Rule` (kind/name/num/refs/body) и `state.DNSServer`/`state.DNSRule`
// (tag + body), и ровно эти записи едут в файле 1.0.
//
// В файле 0.12 секции ехали непрозрачным блоком `ServerSections{Raw}`, куда
// прежний писатель клал ФОРМУ СОСТОЯНИЯ (ловушка CODEMAP §7.6: схема обещала
// форму бэкапа). Писателя сняла волна 2 (legacy_write_012.go больше их не
// пишет), а ЧИТАТЕЛЬ остался: такие файлы уже у пользователей на руках, и
// молча терять их секции нельзя (П3/П6). Блок по-прежнему разбирается тем же
// `state.NodeSections`, что его писал.
//
// Отсев чужих видов записей общий для обоих входов: `preset`/`json` среди
// правил узла и `template`/`preset` среди его DNS-записей недопустимы
// (NODE_SECTIONS.md §1) — такая запись отбрасывается с кодом
// `backup_section_record_dropped`. Раньше это делал `dropForeignKinds` в
// состоянии, и потеря уходила в WarnLog без кода: пользователь импорта о ней
// не узнавал вовсе.
package backup

import (
	"encoding/json"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

// decodeBackupSections переводит блок из файла 0.x в записи состояния.
//
// nodeTag нужен и для сообщений, и для предупреждений: в самих записях он не
// участвует — ссылки на узел живут плейсхолдером `@self`.
func decodeBackupSections(sec *ServerSections, nodeTag string) (*state.NodeSections, []Warning) {
	if sec == nil || len(sec.Raw) == 0 {
		return nil, nil
	}
	var out state.NodeSections
	if err := json.Unmarshal(sec.Raw, &out); err != nil {
		debuglog.WarnLog("backup import: node %q carries sections that cannot be read (%v) — imported without them", nodeTag, err)
		return nil, nil
	}
	return normalizeImportedSections(&out, nodeTag)
}

// sectionsAllowedFor — может ли узел этого вида нести секции вообще.
//
// Только одиночный сервер (state.NormalizeNodeSections, NODE_SECTIONS.md §1):
// у подписки секция принадлежала бы кэшу провайдера, у цепочки и
// провайдерской группы — ссылочной сущности, которой нечего инъецировать.
func sectionsAllowedFor(kind state.SourceKind) bool {
	return kind == state.SourceKindServer
}

// dropSectionsForForeignNode снимает поле `sections` у узла, которому оно не
// положено, и называет потерю тем же кодом, что чужую запись внутри секции.
//
// Раньше поле снимала молча NormalizeNodeSections уже в состоянии: отсев по
// ВИДУ ЗАПИСИ делал импорт (с кодом), а отсев по ВИДУ УЗЛА — состояние (без
// кода), и пользователь, принёсший файл, о второй потере не узнавал вовсе.
// Реестр backup_warnings.json обещает код именно на этот случай («поле
// sections у узла, которому оно не положено»).
//
// Возвращает секции, которые едут дальше: у «чужого» узла — nil.
func dropSectionsForForeignNode(kind state.SourceKind, sec *state.NodeSections, nodeTag string) (*state.NodeSections, []Warning) {
	if sec == nil || sec.IsEmpty() || sectionsAllowedFor(kind) {
		return sec, nil
	}
	return nil, []Warning{{
		Code:   WarnBackupSectionRecordDropped,
		Detail: nodeTag + ": sections",
		Kind:   string(kind),
	}}
}

// normalizeImportedSections снимает записи видов, которых у секции быть не
// может, и называет каждую потерю кодом.
//
// Отбрасывание здесь, а не в `NormalizeNodeSections` (состояние), потому что
// код предупреждения — это разговор с ПОЛЬЗОВАТЕЛЕМ ИМПОРТА: он принёс файл,
// и он обязан узнать, что часть связки узла не применилась. Состояние про
// импорт ничего не знает и говорить может только в лог.
//
// Возвращается тот же объект (записи фильтруются на месте): вызывающий всегда
// передаёт СВОЮ копию — либо только что разобранную из файла, либо Clone().
func normalizeImportedSections(ns *state.NodeSections, nodeTag string) (*state.NodeSections, []Warning) {
	if ns == nil {
		return nil, nil
	}
	var warns []Warning
	drop := func(kind string) {
		warns = append(warns, Warning{
			Code:   WarnBackupSectionRecordDropped,
			Detail: nodeTag + ": " + kind,
			Kind:   kind,
		})
	}

	rules := make([]state.Rule, 0, len(ns.Rules))
	for _, r := range ns.Rules {
		switch r.Kind {
		case state.RuleKindInline, state.RuleKindSrs:
			rules = append(rules, r)
		default:
			// `preset` у узла означал бы ссылку на шаблон, которого на чужой
			// машине может не быть; `json` — сырое правило другой стороны.
			drop(string(r.Kind))
		}
	}
	ns.Rules = nil
	if len(rules) > 0 {
		ns.Rules = rules
	}

	if ns.DNS != nil {
		servers := make([]state.DNSServer, 0, len(ns.DNS.Servers))
		for _, s := range ns.DNS.Servers {
			if s.Kind == state.DNSServerKindUser {
				servers = append(servers, s)
				continue
			}
			drop(string(s.Kind))
		}
		dnsRules := make([]state.DNSRule, 0, len(ns.DNS.Rules))
		for _, r := range ns.DNS.Rules {
			if r.Kind == state.DNSRuleKindUser {
				dnsRules = append(dnsRules, r)
				continue
			}
			drop(string(r.Kind))
		}
		ns.SetDNS(servers, dnsRules)
	}

	if ns.IsEmpty() {
		return nil, warns
	}
	return ns, warns
}
