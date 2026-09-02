package backup

// Экспорт состояния лаунчера в переносимый LX Backup (контракт 0.11.0).

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// ExportOptions — что подмешать в шапку файла.
type ExportOptions struct {
	// AppVersion — версия лаунчера (exported_by.version).
	AppVersion string
	// Platform — GOOS, для диагностики односторонних полей.
	Platform string
	// Now — момент экспорта; ноль означает time.Now(). Параметр существует
	// ради воспроизводимых тестов, а не ради «настраиваемости».
	Now time.Time
}

// Export переносит state в формат бэкапа.
//
// Экспорт — ЧИСТАЯ ФУНКЦИЯ СОСТОЯНИЯ (П1): всё, что пишется в файл, читается
// из полей state, и ничего кроме. Ни блобов «на провоз», ни следов того,
// откуда состояние взялось: два неотличимых состояния обязаны давать
// байт-идентичные файлы, а состояние, приехавшее импортом, — тот же файл,
// что и настроенное руками.
//
// Что НЕ едет и почему:
//   - runtime-данные подписок (Meta: когда обновлялась, сколько нод) —
//     они принадлежат машине, а не пользовательской настройке;
//   - финальные конфиговые теги (П5) — они вычисляются каждой сборкой на
//     принимающей стороне, и хранимый приехал бы мёртвым;
//   - упразднённый detour_node_hash — его нет в схеме 0.11.0.
//
// Пустые и нулевые поля опускаются: значение по умолчанию, записанное явно, —
// это лишний шум, из-за которого «одно и то же состояние» перестаёт давать
// один и тот же файл.
// Второй возврат — предупреждения экспорта: то, что состояние несёт, а
// контракт 0.11 выразить не умеет (папки, провайдерские группы). Молчаливое
// выпадение здесь запрещено: пользователь обязан узнать, что часть настройки
// в файл не поехала, ДО того как восстановится на новой машине. Записи,
// выпавшие ЦЕЛИКОМ, идут в начале списка (SPEC 116 §O1=А) — UI показывает
// его в том же порядке.
func Export(s *state.State, opts ExportOptions) (*Backup, []Warning, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("nil state")
	}
	var warnings []Warning
	// dropped — записи, которых в файле не будет вовсе (папки, провайдерские
	// группы). Копятся отдельно, чтобы встать в начало общего списка.
	var dropped []Warning
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	b := &Backup{
		LxBackup: FormatVersion,
		ExportedBy: ExportedBy{
			App:      AppLauncher,
			Version:  opts.AppVersion,
			Platform: opts.Platform,
		},
		ExportedAt: now.UTC().Format(time.RFC3339),
	}

	// SPEC 104: Направления едут вместе с правилами — иначе правило,
	// сославшееся на `vpn-3`, приезжало бы в никуда.
	for _, d := range s.Directions {
		if d.Tag == "" {
			continue
		}
		b.Directions = append(b.Directions, exportDirection(d))
	}

	// Цепочки — корневая секция chains[], у обеих сторон общая. Порядок
	// записей нормативен (вложенная цепочка объявлена раньше использующей) —
	// сохраняется порядок списка источников.
	//
	// subIndex — порядковый номер ПОДПИСКИ в b.Subscriptions, а не позиция
	// источника в общем списке. Позиционный дериватив тега замены («<N>:»)
	// импорт считает по индексу внутри секции subscriptions[]
	// (importSubscription → backupReplaceTag), и экспорт обязан говорить о
	// том же числе. Пока здесь стоял общий индекс, подписка без префикса,
	// стоящая после сервера, экспортировалась молча: `2:select` совпадал с
	// «деривативом», а импорт восстанавливал `1:select` — правила того же
	// файла повисали, и предупреждения об этом не было.
	subIndex := 0
	for i, src := range s.Sources {
		switch src.Kind {
		case state.SourceKindSubscription:
			b.Subscriptions = append(b.Subscriptions, exportSubscription(src, subIndex))
			// Тег замены — единственное поле v7, у которого в контракте нет
			// дома: 0.11 выводила имя группы формулой из префикса. Явное имя,
			// с формулой не совпавшее, на приёмнике станет другим, и правила
			// этого же файла метят мимо — молчать здесь нельзя.
			if derived, ok := replaceTagSurvivesExport(src, subIndex); !ok {
				warnings = append(warnings, Warning{
					Code:   WarnBackupReplaceTagDerived,
					Detail: sourceExportName(src) + ": " + src.Replace.Tag + " → " + derived,
				})
			}
			// Настройки, которыми подписка представляется провайдеру
			// (UA, HWID и компания), контракт 0.11 не выражает: поля общие
			// с LxBox, а этих у него нет. Заводить их односторонне нельзя,
			// поэтому единственный честный ход — назвать потерю вслух.
			if fields := droppedSourceIdentityFields(src); fields != "" {
				warnings = append(warnings, Warning{
					Code:   WarnBackupSourceIdentityDropped,
					Detail: sourceExportName(src) + ": " + fields,
				})
			}
			subIndex++
		case state.SourceKindServer:
			b.Servers = append(b.Servers, exportServer(src))
		case state.SourceKindChain:
			b.Chains = append(b.Chains, exportChain(src, i))
		case state.SourceKindFolder, state.SourceKindAuto:
			// Контракт 0.11 не знает ни папок, ни провайдерских групп
			// (секция folders[] — отдельный трек с LxBox-стороной, SPEC 118
			// §2 «НЕ в этапе 2»). Молча выронить их из файла нельзя: это
			// ровно та ловушка, из-за которой SPEC 116 потерял бы состав
			// папки без единого слова.
			//
			// SPEC 116 W9 (§O1=А): вид и объём — отдельными полями. Папка
			// уезжает НЕ ОДНА, а со своим составом, и пользователю нужно
			// число: «папка Рабочая и её 12 узлов в файл не попали» — это
			// решение не восстанавливаться из этого файла, а «папка не
			// поддержана» — нет.
			dropped = append(dropped, Warning{
				Code:   WarnBackupSourceKindUnsupported,
				Detail: sourceExportName(src),
				Kind:   string(src.Kind),
				Nodes:  len(src.Nodes),
			})
		}
	}

	// Потерянные целиком записи — В НАЧАЛО списка (§O1=А, «первой строкой»).
	// Прочие предупреждения экспорта говорят «приехало иначе»; эти — «не
	// приехало вовсе», и утонуть под двумя десятками переименованных тегов
	// замены они не должны: сортировка здесь — не косметика, а разница между
	// «прочитал и передумал восстанавливаться» и «узнал после restore».
	warnings = append(dropped, warnings...)

	for _, r := range s.Rules {
		rule, err := exportRule(r)
		if err != nil {
			return nil, warnings, fmt.Errorf("rule %s: %w", r.Kind, err)
		}
		b.Rules = append(b.Rules, rule)
	}

	if vars := exportVars(s.Vars); len(vars) > 0 {
		b.Vars = vars
	}
	if final := routeFinal(s); final != "" {
		b.Route = &Route{Final: final}
	}
	if dns := exportDNS(s); dns != nil {
		b.DNS = dns
	}

	// Warp-аккаунты (BACKUP.md §2): канонические snake_case-поля плюс
	// дискриминатор type. Без этого регистрация не переезжала вовсе, и на
	// новой машине «Add WARP» плодил лишние device-записи в Cloudflare.
	b.Warp = exportWarp(s)

	return b, warnings, nil
}

// exportDirections — список Направлений в канонической форме.
func exportDirections(list []configtypes.Direction) []Direction {
	var out []Direction
	for _, d := range list {
		if d.Tag == "" {
			continue
		}
		out = append(out, exportDirection(d))
	}
	return out
}

// exportSourceRef — ссылка источника на цель дозвона.
//
// Упразднённый detour_node_hash не пишется никогда: в схеме 0.11.0 его нет.
// Источник, ещё не прошедший миграцию, отдаёт пустую ссылку — она
// восстановится на приёмнике первой же сборкой из своего состояния, а вывозить
// протухающий хеш значило бы вернуть в формат ровно то, ради чего он снесён.
func exportSourceRef(src state.Source) SourceRef {
	return exportNodeLinkRef(src.Detour)
}

// sourceExportName — как назвать источник в предупреждении экспорта.
func sourceExportName(src state.Source) string {
	if src.Name != "" {
		return src.Name
	}
	if n := src.NodeTagOrLabel(); n != "" {
		return n
	}
	return src.ID
}

// exportChain — запись секции chains[] (SPEC 110).
//
// Tag — эффективный тег outbound'а цепочки (NodeTagOrLabel), на который
// ссылаются правила и позиции других цепочек.
// index — позиция источника в общем списке: у безымянной цепочки тег на
// сборке получается позиционным (chainSourceTag), и в файл обязан уехать тот
// же эффективный тег — схема требует tag, а импорт зафиксирует его именем
// (нормализация той же категории, что перенумерация правил).
//
// `label` контракта остаётся ПУСТЫМ и заполняться не будет: у канона v7 имя
// узла одно — тег (SPEC 112, «идентичность узла = тег»), второго источника
// подписи в состоянии нет. Поле живо в схеме ради LxBox-стороны; писать в
// него копию тега значило бы гонять шум, который импорт всё равно отбросит.
func exportChain(src state.Source, index int) Chain {
	out := Chain{
		ID:    src.ID,
		Tag:   src.NodeTagOrLabel(),
		Chain: exportChainSpec(src),
	}
	if out.Tag == "" {
		out.Tag = "chain-" + strconv.Itoa(index+1)
	}
	if !src.Enabled {
		out.Enabled = boolPtr(false)
	}
	return out
}

// exportWarp — WG/MASQUE-регистрации в переносимую форму warp[].
func exportWarp(s *state.State) []json.RawMessage {
	if s.WarpAccounts == nil {
		return nil
	}
	var out []json.RawMessage
	appendAcc := func(typ string, acc any) {
		m := map[string]any{}
		raw, err := json.Marshal(acc)
		if err != nil || json.Unmarshal(raw, &m) != nil {
			return
		}
		m["type"] = typ
		if enc, err := json.Marshal(m); err == nil {
			out = append(out, enc)
		}
	}
	if s.WarpAccounts.WG != nil {
		appendAcc("wg", s.WarpAccounts.WG)
	}
	if s.WarpAccounts.Masque != nil {
		appendAcc("masque", s.WarpAccounts.Masque)
	}
	return out
}

func exportSubscription(src state.Source, index int) Subscription {
	out := Subscription{
		ID:        src.ID,
		URL:       src.URL,
		Label:     src.Name,
		MaxNodes:  src.MaxNodes,
		Skip:      src.Skip,
		Fold:      exportFold(src.Replace),
		Disabled:  exportDisabledMap(src),
		SourceRef: exportSourceRef(src),
	}
	if out.Label == "" {
		out.Label = src.Label
	}
	if !src.Enabled {
		out.Enabled = boolPtr(false)
	}
	if src.TagPolicy != nil && !src.TagPolicy.IsZero() {
		out.Tag = &TagPolicy{Prefix: src.TagPolicy.Prefix, Postfix: src.TagPolicy.Postfix}
	}
	if src.Update != nil {
		out.Update = &UpdatePolicy{IntervalHours: src.Update.IntervalHours, Auto: src.Update.AutoRefresh}
	}
	return out
}

// droppedSourceIdentityFields — перечень per-source настроек подписки,
// которые в контракт 0.11 не едут, в порядке объявления в модели
// (core/state/sources_v7.go). Пусто = терять нечего.
//
// Это НЕ «поля, о которых забыли»: UA и HWID описывают, каким устройством
// подписка представляется провайдеру, а провайдер ветвит выдачу по ним
// (одна ссылка отдаёт разным клиентам разные тела). Восстановившись из
// бэкапа, пользователь получит ДРУГОЙ набор узлов и не поймёт, почему —
// поэтому потеря обязана быть названа поимённо, а не общим «часть настроек».
func droppedSourceIdentityFields(src state.Source) string {
	var fields []string
	if strings.TrimSpace(src.UserAgent) != "" {
		fields = append(fields, "user_agent")
	}
	if strings.TrimSpace(src.HWID) != "" {
		fields = append(fields, "hwid")
	}
	if src.SendHWID != nil {
		fields = append(fields, "send_hwid")
	}
	if src.HashDeviceModel != nil {
		fields = append(fields, "hash_device_model")
	}
	if src.RelaysInDirections {
		fields = append(fields, "relays_in_directions")
	}
	return strings.Join(fields, ", ")
}

// exportServer — запись секции servers[].
//
// `label` контракта не заполняется (та же причина, что у exportChain): имя
// одиночного узла в v7 — его тег, и он уезжает ключом node_tag.
func exportServer(src state.Source) Server {
	out := Server{
		ID:        src.ID,
		NodeTag:   src.Tag,
		SourceRef: exportSourceRef(src),
	}
	// Форма хранения узла в v7 одна — тело; исходный URI живёт в origin и
	// едет тем же ключом, что и раньше, когда он был единственной формой.
	//
	// Блок wg-quick едет ТЕМ ЖЕ ключом `uri`: контракт (общий с LxBox) знает
	// только uri/config_json, а материализация принимает и ссылку, и текст
	// [Interface]/[Peer] — распознаёт по форме и возвращает origin.kind
	// wg_ini. Без этой ветки INI-узел экспортировался телом, и на импорте
	// исходник со всеми комментариями (включая имя пира) исчезал: узел терял
	// «Regen from raw» и приезжал безымянным.
	if src.Origin != nil &&
		(src.Origin.Kind == state.OriginKindURI || src.Origin.Kind == state.OriginKindWGIni) {
		out.URI = src.Origin.Raw
	} else if len(src.Body) > 0 {
		out.ConfigJSON = append(json.RawMessage(nil), src.Body...)
	}
	if !src.Enabled {
		out.Enabled = boolPtr(false)
	}
	return out
}

func exportRule(r state.Rule) (Rule, error) {
	out := Rule{
		Kind:    RuleKind(r.Kind),
		Enabled: boolPtr(r.Enabled),
	}
	if r.OrderNum != nil {
		out.Num = f64Ptr(float64(*r.OrderNum))
	}
	if !r.Enabled {
		out.Enabled = boolPtr(false)
	} else {
		out.Enabled = nil // true — умолчание схемы
	}

	switch r.Kind {
	case state.RuleKindPreset:
		out.Ref = r.Ref
		var body state.PresetBody
		if len(r.Body) > 0 {
			if err := json.Unmarshal(r.Body, &body); err != nil {
				return Rule{}, fmt.Errorf("preset body: %w", err)
			}
		}
		if len(body.Vars) > 0 {
			out.Vars = body.Vars
		}
	case state.RuleKindInline:
		var body state.InlineBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			return Rule{}, fmt.Errorf("inline body: %w", err)
		}
		out.Name = body.Name
		out.Outbound = body.Outbound
		if len(body.Match) > 0 {
			raw, err := json.Marshal(body.Match)
			if err != nil {
				return Rule{}, fmt.Errorf("inline match: %w", err)
			}
			out.Match = raw
		}
	case state.RuleKindSrs:
		var body state.SrsBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			return Rule{}, fmt.Errorf("srs body: %w", err)
		}
		out.Name = body.Name
		out.Ref = body.SrsURL
		out.Outbound = body.Outbound
	default:
		return Rule{}, fmt.Errorf("unknown kind %q", r.Kind)
	}
	return out, nil
}

// exportVars отдаёт только переносимые имена переменных.
//
// Реестр (registry/vars.json) помечает portable-имена: те, что означают одно
// и то же на обеих сторонах. Непереносимое (пути, интерфейсы, платформенные
// флаги) на другой машине значит другое, и переносить его — значит молча
// сломать чужую настройку.
func exportVars(vars []state.SettingVar) map[string]string {
	if len(vars) == 0 {
		return nil
	}
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		if !IsPortableVar(v.Name) {
			continue
		}
		out[v.Name] = v.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func routeFinal(s *state.State) string {
	for _, p := range s.ConfigParams {
		if p.Name == "final" || p.Name == "route.final" {
			return p.Value
		}
	}
	return ""
}

// exportDNS переносит секцию DNS.
//
// Записи kind=template/preset едут ССЫЛКАМИ (ref/tag), а не телом: тело
// приедет из шаблона принимающей стороны. Переносить развёрнутое тело значило
// бы зафиксировать чужие умолчания навсегда — обновление шаблона перестало бы
// доходить до пользователя.
func exportDNS(s *state.State) *DNS {
	out := &DNS{
		Final:    s.DNS.Final,
		Strategy: s.DNS.Strategy,
	}
	for _, srv := range s.DNS.Servers {
		out.Servers = append(out.Servers, dnsRefFrom(string(srv.Kind), srv.Tag, srv.Ref, srv.Enabled, srv.Body))
	}
	for _, rule := range s.DNS.Rules {
		out.Rules = append(out.Rules, dnsRefFrom(string(rule.Kind), "", rule.Ref, rule.Enabled, rule.Body))
	}
	if out.Final == "" && out.Strategy == "" && len(out.Servers) == 0 && len(out.Rules) == 0 {
		return nil
	}
	return out
}

func dnsRefFrom(kind, tag, ref string, enabled bool, body map[string]interface{}) DNSRef {
	out := DNSRef{Kind: kind, Name: tag, Ref: ref}
	if !enabled {
		out.Enabled = boolPtr(false)
	}
	// Тело переносится только у пользовательских записей: у template/preset
	// оно принадлежит шаблону принимающей стороны.
	if kind == "user" && len(body) > 0 {
		if raw, err := json.Marshal(body); err == nil {
			out.Value = raw
		}
	}
	return out
}
