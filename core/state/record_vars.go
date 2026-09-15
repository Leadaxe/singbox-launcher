// File record_vars.go — значения переменных записи (SPEC 129): шаблонный
// DNS-сервер `dns.servers[kind=template].vars` и пресет правила
// `rules[kind=preset].vars`.
//
// # Нормы (SPEC 129 §3)
//
//   - Н1: `vars` — объект string→string; пустой объект не пишется.
//   - Н2: законны только имена, объявленные носителем в шаблоне своей
//     стороны; необъявленное снимается (импорт называет его, хранение —
//     молча). Запись шаблонного сервера, чей тег шаблон не объявил вовсе, —
//     сирота и снимается целиком.
//   - Н3: значение хранится подрезанным, пустое = нет ключа.
//   - Н4: значение, равное умолчанию объявления, не пишется.
//   - Н8: корневые `vars.dns_<tag>_<var>` (форма до SPEC 129) переносятся в
//     запись сервера; кандидаты — объявленные пары (tag, var), выигрывает
//     самый длинный тег.
//
// # Почему здесь, а не в core/template
//
// Пакет state о шаблоне не знает намеренно (rule_order.go: RuleOrderSpec), а
// core/backup получает шаблон только опциями. Поэтому объявления приходят
// сюда формой без типов шаблона — RecordVarDecls с умолчанием, уже
// разрешённым для цели состояния; заполняет её мост
// template.RecordVarDeclsFor. Одна функция на все точки, где состояние
// встречается с шаблоном: загрузка в UI, сборка из файла, сохранение из UI,
// экспорт, импорт, debug API.
package state

import (
	"sort"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// RecordVarDecl — объявление переменной записи.
type RecordVarDecl struct {
	// Name — имя в объявлении носителя (локальное: `outbound`, `dns_ip`).
	Name string
	// Type — тип объявления (`outbound`, `dns_server`, `enum`, …).
	Type string
	// Default — умолчание, разрешённое для цели состояния; "" — умолчания
	// нет, и снимать по Н4 нечего.
	Default string
	// Ref — переменная пресета со ссылкой на глобальную (`ref`): её значение
	// живёт в глобальных vars, а хранимое в записи читают резолвер DNS и
	// outbounds пресета — нормализация такое имя не трогает (Н2).
	Ref bool
}

// RecordVarDecls — объявления шаблона стороны для обоих носителей.
type RecordVarDecls struct {
	// DNSServers — шаблонные DNS-серверы по тегу. Ключ есть — сервер объявлен
	// шаблоном (список пуст у сервера без переменных). nil — шаблона нет:
	// записи серверов не нормализуются и сироты не снимаются.
	DNSServers map[string][]RecordVarDecl
	// DNSServerEnabled — включённость по умолчанию серверов шаблона: с ней
	// создаётся запись, когда корневое имя нашло объявленный сервер, у
	// которого записи ещё нет (Н8, своё хранение).
	DNSServerEnabled map[string]bool
	// Presets — пресеты шаблона по id. nil — пресеты не нормализуются; id,
	// которого здесь нет, — тоже: пресет мог отфильтроваться платформой
	// хоста, а у удалённой машины и мобильного его может не быть вовсе.
	Presets map[string][]RecordVarDecl
}

// Причины снятия имени — значения `reason` кода backup_var_skipped
// (contract/registry/backup_warnings.json). `not_portable` ставит импорт
// корневых переменных (core/backup), здесь его нет.
const (
	RecordVarUndeclared = "undeclared"
	RecordVarSuperseded = "superseded"
	RecordVarNoRecord   = "no_record"
)

// RecordVarDrop — имя, снятое с отчётом (импорт называет его предупреждением).
type RecordVarDrop struct {
	// Name — имя переменной: локальное у записи, корневое у корневой.
	Name string
	// Record — носитель: DNSVarRecord / PresetVarRecord; пусто у корневой.
	Record string
	// Reason — RecordVarUndeclared | RecordVarSuperseded | RecordVarNoRecord.
	Reason string
}

// DNSVarRecord — значение `record` для шаблонного DNS-сервера.
func DNSVarRecord(tag string) string { return "dns:" + tag }

// PresetVarRecord — значение `record` для пресета правила.
func PresetVarRecord(ref string) string { return "preset:" + ref }

// DNSServerDeclared — объявлен ли сервер шаблоном. Без шаблона (decls nil)
// ответить нечем — сервер считается объявленным, записи не трогаются.
func (d *RecordVarDecls) DNSServerDeclared(tag string) bool {
	if d == nil || d.DNSServers == nil {
		return true
	}
	_, ok := d.DNSServers[tag]
	return ok
}

// DNSServerVarDecl — объявление переменной сервера по имени.
func (d *RecordVarDecls) DNSServerVarDecl(tag, name string) (RecordVarDecl, bool) {
	if d == nil {
		return RecordVarDecl{}, false
	}
	for _, v := range d.DNSServers[tag] {
		if v.Name == name {
			return v, true
		}
	}
	return RecordVarDecl{}, false
}

// MatchRootDNSVar разрешает корневое имя `dns_<tag>_<var>` против объявлений
// (Н8): кандидаты — объявленные пары (tag, var), чьё склеенное имя равно
// name; из нескольких выигрывает самый длинный тег. Тег с подчёркиванием
// делает склейку неоднозначной не только между тегами, но и между парами:
// `google_doh` с `vpn_outbound` и `google_doh_vpn` с `outbound` дают одно имя.
func (d *RecordVarDecls) MatchRootDNSVar(name string) (tag, local string, ok bool) {
	if d == nil || !strings.HasPrefix(name, "dns_") {
		return "", "", false
	}
	rest := name[len("dns_"):]
	candidates := 0
	for t, vars := range d.DNSServers {
		if t == "" || !strings.HasPrefix(rest, t+"_") {
			continue
		}
		v := rest[len(t)+1:]
		declared := false
		for _, decl := range vars {
			if decl.Name == v {
				declared = true
				break
			}
		}
		if !declared {
			continue
		}
		candidates++
		// Самый длинный тег; при равной длине — меньший по строке, чтобы
		// ответ не зависел от обхода карты.
		if len(t) > len(tag) || (len(t) == len(tag) && t < tag) {
			tag, local = t, v
		}
	}
	if candidates > 1 {
		debuglog.WarnLog("state: root variable %q matches %d declared DNS server variables — the template is ambiguous; %q/%q wins (longest tag)",
			name, candidates, tag, local)
	}
	return tag, local, tag != ""
}

// ApplyRecordVars приводит состояние к нормам SPEC 129 на своём хранении:
// переносит корневые `dns_<tag>_<var>` в записи серверов (Н8, записи нет —
// создаётся), снимает сирот, необъявленные имена, пустые и равные умолчанию
// значения у обоих носителей — молча (Н2–Н4).
//
// decls nil — шаблона нет, состояние не трогается. Возвращает true, если
// что-то изменилось.
func ApplyRecordVars(s *State, decls *RecordVarDecls) bool {
	if s == nil || decls == nil {
		return false
	}
	changed := false

	if len(s.Vars) > 0 && len(decls.DNSServers) > 0 {
		root := make([]RootVar, 0, len(s.Vars))
		for _, v := range s.Vars {
			root = append(root, RootVar{Name: v.Name, Value: v.Value})
		}
		kept, _, moved := MoveRootDNSVars(root, &s.DNS.Servers, decls, true)
		if moved {
			changed = true
			out := make([]SettingVar, 0, len(kept))
			for _, v := range kept {
				out = append(out, SettingVar{Name: v.Name, Value: v.Value})
			}
			s.Vars = out
		}
	}

	servers, dnsChanged, _ := NormalizeDNSServerVars(s.DNS.Servers, decls)
	if dnsChanged {
		s.DNS.Servers = servers
		changed = true
	}
	if NormalizePresetRuleVars(s.Rules, decls, nil) {
		changed = true
	}
	return changed
}

// RootVar — корневая переменная парой (порядок значим: при повторе имени
// выигрывает последняя, как у загрузки состояния).
type RootVar struct {
	Name  string
	Value string
}

// MoveRootDNSVars — Н8: корневые `dns_<tag>_<var>` в `vars` записей серверов.
//
// Правила (SPEC 129 Н8):
//   - кандидата нет — имя остаётся корневым (kept);
//   - у записи сервера непустой `vars` (снимок ДО переноса) — побеждает
//     запись, корневое снимается: RecordVarSuperseded;
//   - запись есть, `vars` пуст — значение переносится;
//   - записи нет: createMissing (своё хранение) — запись создаётся с
//     включённостью по умолчанию шаблона; иначе (файл) — RecordVarNoRecord.
//
// Снимок нужен потому, что имена переносятся по одному: запись, получившая
// первое имя сервера, иначе выглядела бы «со своими vars» для второго.
//
// Возвращает оставшиеся корневые, снятые с отчётом и признак «хоть одно имя
// разрешено» (перенесено или снято).
func MoveRootDNSVars(root []RootVar, servers *[]DNSServer, decls *RecordVarDecls, createMissing bool) ([]RootVar, []RecordVarDrop, bool) {
	if decls == nil || len(decls.DNSServers) == 0 || len(root) == 0 || servers == nil {
		return root, nil, false
	}
	hadVars := map[string]bool{}
	for _, srv := range *servers {
		if srv.Kind == DNSServerKindTemplate && hasRecordValues(srv.Vars) {
			hadVars[srv.Tag] = true
		}
	}
	var (
		kept    = make([]RootVar, 0, len(root))
		drops   []RecordVarDrop
		touched bool
	)
	for _, rv := range root {
		tag, local, ok := decls.MatchRootDNSVar(rv.Name)
		if !ok {
			kept = append(kept, rv)
			continue
		}
		touched = true
		idx := -1
		for i := range *servers {
			if (*servers)[i].Kind == DNSServerKindTemplate && (*servers)[i].Tag == tag {
				idx = i
				break
			}
		}
		value := strings.TrimSpace(rv.Value)
		switch {
		case hadVars[tag]:
			drops = append(drops, RecordVarDrop{Name: rv.Name, Reason: RecordVarSuperseded})
		case idx < 0 && !createMissing:
			drops = append(drops, RecordVarDrop{Name: rv.Name, Reason: RecordVarNoRecord})
		case value == "":
			// Пустое значение — «нет ключа» (Н3): переносить нечего.
		case idx < 0:
			enabled, known := decls.DNSServerEnabled[tag]
			if !known {
				enabled = true
			}
			*servers = append(*servers, DNSServer{
				Kind:    DNSServerKindTemplate,
				Tag:     tag,
				Enabled: enabled,
				Vars:    map[string]string{local: value},
			})
		default:
			srv := &(*servers)[idx]
			if srv.Vars == nil {
				srv.Vars = map[string]string{}
			}
			srv.Vars[local] = value
		}
	}
	return kept, drops, touched
}

// MoveRootDNSVarsMap — MoveRootDNSVars для карты корневых vars файла:
// перенесённые и снятые имена удаляются из карты. Имена обходятся по
// алфавиту — отчёт не зависит от обхода карты.
func MoveRootDNSVarsMap(vars map[string]string, servers *[]DNSServer, decls *RecordVarDecls, createMissing bool) []RecordVarDrop {
	if len(vars) == 0 {
		return nil
	}
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	root := make([]RootVar, 0, len(names))
	for _, name := range names {
		root = append(root, RootVar{Name: name, Value: vars[name]})
	}
	kept, drops, touched := MoveRootDNSVars(root, servers, decls, createMissing)
	if !touched {
		return drops
	}
	keep := make(map[string]bool, len(kept))
	for _, rv := range kept {
		keep[rv.Name] = true
	}
	for _, name := range names {
		if !keep[name] {
			delete(vars, name)
		}
	}
	return drops
}

// NormalizeDNSServerVars — Н2–Н4 для записей шаблонных серверов плюс снятие
// сирот (тег не объявлен шаблоном). Возвращает список (новый срез, если что-то
// менялось), признак изменения и необъявленные имена — импорт называет их
// предупреждением, хранение отбрасывает отчёт.
func NormalizeDNSServerVars(servers []DNSServer, decls *RecordVarDecls) ([]DNSServer, bool, []RecordVarDrop) {
	if decls == nil || decls.DNSServers == nil || len(servers) == 0 {
		return servers, false, nil
	}
	var (
		out     []DNSServer
		changed bool
		drops   []RecordVarDrop
	)
	for i, srv := range servers {
		if srv.Kind != DNSServerKindTemplate {
			if out != nil {
				out = append(out, srv)
			}
			continue
		}
		declared, ok := decls.DNSServers[srv.Tag]
		if !ok {
			// Сирота (§4.4): тела в шаблоне нет, сборка её не эмитит.
			if out == nil {
				out = append([]DNSServer(nil), servers[:i]...)
			}
			changed = true
			continue
		}
		vars, varsChanged, d := NormalizeRecordVarMap(srv.Vars, declared, DNSVarRecord(srv.Tag))
		drops = append(drops, d...)
		if varsChanged {
			if out == nil {
				out = append([]DNSServer(nil), servers[:i]...)
			}
			srv.Vars = vars
			changed = true
		}
		if out != nil {
			out = append(out, srv)
		}
	}
	if !changed {
		return servers, false, drops
	}
	return out, true, drops
}

// NormalizePresetRuleVars — Н2–Н4 для `vars` записей пресетов (правила
// меняются на месте). Пресет, которого объявления не знают, не трогается.
// report — куда складывать необъявленные имена (nil — молча). Возвращает
// true, если что-то изменилось.
func NormalizePresetRuleVars(rules []Rule, decls *RecordVarDecls, report *[]RecordVarDrop) bool {
	if decls == nil || decls.Presets == nil {
		return false
	}
	changed := false
	for i := range rules {
		r := &rules[i]
		if r.Kind != RuleKindPreset || len(r.Vars) == 0 {
			continue
		}
		declared, ok := decls.Presets[r.Ref]
		if !ok {
			continue
		}
		vars, varsChanged, d := NormalizeRecordVarMap(r.Vars, declared, PresetVarRecord(r.Ref))
		if report != nil {
			*report = append(*report, d...)
		}
		if varsChanged {
			r.Vars = vars
			changed = true
		}
	}
	return changed
}

// NormalizeRecordVarMap — Н2–Н4 для одной карты значений против объявлений
// носителя. Возвращает карту (nil, если пусто), признак изменения и
// необъявленные имена (по алфавиту). Переменная с `ref` проносится как есть.
func NormalizeRecordVarMap(vars map[string]string, declared []RecordVarDecl, record string) (map[string]string, bool, []RecordVarDrop) {
	if len(vars) == 0 {
		return nil, vars != nil, nil
	}
	byName := make(map[string]RecordVarDecl, len(declared))
	for _, d := range declared {
		byName[d.Name] = d
	}
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make(map[string]string, len(vars))
	changed := false
	var drops []RecordVarDrop
	for _, name := range names {
		raw := vars[name]
		decl, ok := byName[name]
		if !ok {
			drops = append(drops, RecordVarDrop{Name: name, Record: record, Reason: RecordVarUndeclared})
			changed = true
			continue
		}
		if decl.Ref {
			// Хранимое значение ref-переменной читают резолвер DNS и outbounds
			// пресета — не трогается, кроме пустого: пустое = нет ключа (Н3).
			if strings.TrimSpace(raw) == "" {
				changed = true
				continue
			}
			out[name] = raw
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" || (decl.Default != "" && value == decl.Default) {
			changed = true
			continue
		}
		if value != raw {
			changed = true
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil, true, drops
	}
	return out, changed, drops
}

// UndeclaredRecordVars — необъявленные имена карты без её изменения: импорт
// называет их по записи ФАЙЛА до наложения на запись приёмника (§5.2), а
// нормализует уже результат наложения — молча.
func UndeclaredRecordVars(vars map[string]string, declared []RecordVarDecl, record string) []RecordVarDrop {
	if len(vars) == 0 {
		return nil
	}
	byName := make(map[string]bool, len(declared))
	for _, d := range declared {
		byName[d.Name] = true
	}
	names := make([]string, 0, len(vars))
	for name := range vars {
		if !byName[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]RecordVarDrop, 0, len(names))
	for _, name := range names {
		out = append(out, RecordVarDrop{Name: name, Record: record, Reason: RecordVarUndeclared})
	}
	return out
}

// hasRecordValues — есть ли в карте хоть одно непустое значение (Н1/Н3:
// пустая карта и пустые значения равны отсутствию `vars`).
func hasRecordValues(vars map[string]string) bool {
	for _, v := range vars {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}
