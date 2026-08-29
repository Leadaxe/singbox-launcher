package business

// clone_source.go — перенос состояния между машинами («Clone from…»).
//
// Задача, которой раньше не было ответа: у пользователя настроена одна
// машина, и он заводит вторую. Read показывает снапшоты ТОЛЬКО своей машины
// (изоляция store'ов, см. NewStateStoreFor), LX Backup ходит через файл на
// диске — а между двумя машинами ОДНОГО лаунчера файл посередине не нужен:
// оба состояния лежат в bin/wizard_states/, читать их можно напрямую.
//
// Что переносится и что нет — не вопрос вкуса, а вопрос того, останется ли
// склонированный конфиг рабочим:
//
//   - смысловая начинка (источники, Направления, цепочки, правила, DNS,
//     WARP-регистрации) описывает ЧЕГО пользователь хочет от трафика и не
//     зависит от того, на каком железе крутится ядро — переносится целиком;
//   - привязки к СУЩЕСТВУЮЩЕМУ на машине железу (LAN-интерфейсы шлюза,
//     исходящий интерфейс) значимы только на своей машине: br-lan роутера на
//     VPS не существует, и конфиг с ним не поднимется — НЕ переносятся, у
//     приёмника остаются свои. Имя TUN сюда НЕ входит: его придумывает
//     пользователь, ядро создаёт интерфейс само, и одинаковое имя на всех
//     машинах — обычно цель, а не помеха;
//   - платформа (GOOS/GOARCH) вообще не живёт в состоянии как настройка: это
//     свойство машины из реестра (SPEC 098 §2.4), клон её не касается.
//
// Почему свой список machine-bound, а не backup.IsPortableVar: тот список
// отвечает на ДРУГОЙ вопрос — «есть ли у переменной пара на мобильном
// LxBox». Половина его portable=false (strict_route, cert_store, proxy_in_*)
// непереносима только из-за разрыва N1 в контракте, а между двумя desktop-
// машинами эти настройки переносятся прекрасно. Отфильтровать клон по
// мобильному списку значило бы молча терять настройки пользователя.

import (
	"fmt"
	"sort"
	"strings"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// machineBoundVars — переменные, чьё значение осмысленно только на той
// машине, где задано. Клон их не переносит: у приёмника остаётся своё.
//
// Список намеренно короткий и обоснован дословно реестром
// (contract/registry/vars.json). Всё, что не здесь, переносится: молча
// потерять настройку хуже, чем перенести лишнюю, которую видно в UI.
var machineBoundVars = map[string]string{
	// «непереносима по определению — имя интерфейса (en0/eth0) значимо
	// только на своей машине» (реестр, дословно).
	"bind_interface": "outbound interface",
	// Имя TUN-интерфейса (tun_interface_name / tun_name) здесь СОЗНАТЕЛЬНО
	// нет, хотя реестр и помечает его portable=false. Это имя выдумывает
	// пользователь, а не машина: оно ничего не обязано «найти» на той
	// стороне — ядро создаёт интерфейс с таким именем само. Держать его
	// одинаковым на всех машинах обычно и есть цель (одни и те же правила
	// firewall, одни и те же скрипты), поэтому клон его переносит. Отличать
	// имя там, где это нужно, пользователь может руками на вкладке Target.
	// Режим шлюза и его LAN-интерфейсы: br-lan роутера на VPS не существует.
	"gateway_mode":              "gateway mode",
	"gateway_include_interface": "gateway LAN interfaces",
	// Clash API: адрес слушателя и его секрет принадлежат конкретному хосту,
	// причём на remote он вообще не генерируется (только local-target).
	"clash_api":    "Clash API address",
	"clash_secret": "Clash API secret",
}

// CloneSourceKind — вид источника клонирования.
type CloneSourceKind int

const (
	// CloneSourceLocal — локальная машина (bin/wizard_states/state.json).
	CloneSourceLocal CloneSourceKind = iota
	// CloneSourceRemote — удалённая машина (bin/wizard_states/remote/<id>/).
	CloneSourceRemote
)

// CloneSource — одна строка списка «откуда клонировать».
type CloneSource struct {
	Kind CloneSourceKind
	// MachineID — пусто для local.
	MachineID string
	// Name — что показывать пользователю («Local», «Home»).
	Name string
	// Platform — «linux/arm64»; пусто, если неизвестна. Справочно: клон
	// платформу не переносит, но она объясняет, откуда берётся конфиг.
	Platform string
	// HasState — есть ли у источника сохранённое состояние. Источник без
	// состояния показывается неактивным, а не прячется: «машина есть, но
	// клонировать с неё нечего» — это ответ, а исчезнувшая строка — загадка.
	HasState bool
}

// Target возвращает пару (target, machineID) для NewStateStoreFor.
func (s CloneSource) Target() (string, string) {
	if s.Kind == CloneSourceRemote {
		return constants.ConfigTargetRemote, s.MachineID
	}
	return constants.ConfigTargetLocal, ""
}

// MachineLister — реестр машин; интерфейс вместо прямой зависимости на
// core/services, чтобы business не тянул сетевой слой ради имён.
type MachineLister interface {
	ListMachines() []CloneSource
}

// ListCloneSources собирает список источников, ИСКЛЮЧАЯ текущий: клон
// самого себя — это no-op, а строка, которая ничего не делает, читается как
// сломанная.
//
// Порядок: local первым (самый частый донор — там настроено раньше всего),
// затем машины по имени.
func ListCloneSources(execDir string, machines []CloneSource, curTarget, curMachineID string) []CloneSource {
	curTarget = strings.ToLower(strings.TrimSpace(curTarget))
	curIsRemote := curTarget == constants.ConfigTargetRemote

	var out []CloneSource

	// Local в списке всегда, кроме случая «мы сами local»: клон самого себя
	// ничего не делает, а строка, которая ничего не делает, читается как
	// сломанная.
	if curIsRemote {
		out = append(out, CloneSource{
			Kind:     CloneSourceLocal,
			Name:     "Local",
			HasState: stateExistsFor(execDir, constants.ConfigTargetLocal, ""),
		})
	}

	sort.Slice(machines, func(i, j int) bool { return machines[i].Name < machines[j].Name })
	for _, m := range machines {
		if m.MachineID == "" {
			continue
		}
		if curIsRemote && m.MachineID == curMachineID {
			continue // сам себе не источник
		}
		m.Kind = CloneSourceRemote
		m.HasState = stateExistsFor(execDir, constants.ConfigTargetRemote, m.MachineID)
		out = append(out, m)
	}
	return out
}

// stateExistsFor — есть ли у источника state.json.
func stateExistsFor(execDir, target, machineID string) bool {
	path := platform.GetWizardStatePathFor(execDir, target, machineID)
	_, err := corestate.Load(path)
	return err == nil
}

// CloneSummary — что приедет; показывается ДО применения.
type CloneSummary struct {
	Subscriptions int
	Servers       int
	Chains        int
	Directions    int
	Rules         int
	Vars          int
	// SkippedVars — machine-bound переменные, которые НЕ переносятся, с
	// подписями (ключи локализации). Предъявляются пользователю: молчаливая
	// потеря настройки — тот же баг, что и молчаливый перенос чужой.
	SkippedVars []string
}

// LoadCloneState читает состояние источника и готовит его к применению на
// текущей машине: чистит machine-bound переменные и снимает идентичность
// донора (ID/Comment/Target), чтобы клон не выдавал себя за исходный файл.
//
// Возвращает состояние и сводку. Состояние НЕ записывается на диск —
// решение применять принимает вызывающий после подтверждения.
func LoadCloneState(execDir string, src CloneSource) (*corestate.State, CloneSummary, error) {
	target, machineID := src.Target()
	path := platform.GetWizardStatePathFor(execDir, target, machineID)

	st, err := corestate.Load(path)
	if err != nil {
		return nil, CloneSummary{}, fmt.Errorf("clone: read %s: %w", src.Name, err)
	}

	summary := scrubMachineBound(st)

	// Идентичность донора не едет: ID — имя снапшота, Comment — его
	// описание, Target — чья это машина. Всё три переставит приёмник.
	st.ID = ""
	st.Comment = ""
	st.Target = ""
	st.TargetPlatform = ""
	st.TargetArch = ""

	debuglog.InfoLog("clone: loaded state from %s (%s): %d sources, %d rules, %d vars, %d machine-bound skipped",
		path, src.Name, len(st.Sources), len(st.Rules), len(st.Vars), len(summary.SkippedVars))

	return st, summary, nil
}

// scrubMachineBound удаляет machine-bound переменные и считает сводку.
func scrubMachineBound(st *corestate.State) CloneSummary {
	var sum CloneSummary

	for _, s := range st.Sources {
		switch s.Kind {
		case corestate.SourceTypeSubscription:
			sum.Subscriptions++
		case corestate.SourceTypeServer:
			sum.Servers++
		case corestate.SourceTypeChain:
			// Цепочка — тоже источник: она ВЕДЁТ маршрут, а Направление
			// выбирает между маршрутами. Поэтому считается здесь.
			sum.Chains++
		}
	}
	sum.Directions = len(st.Directions)
	// Считаем по Rules и только по ним: corestate.Load наполняет эту секцию
	// всегда — и из v6-файла, и деривацией из legacy custom_rules
	// (deriveV6FromLegacy). Фолбэк на CustomRules был бы недостижимым кодом.
	sum.Rules = len(st.Rules)

	kept := make([]corestate.SettingVar, 0, len(st.Vars))
	seen := map[string]struct{}{}
	for _, v := range st.Vars {
		if label, bound := machineBoundVars[v.Name]; bound {
			if _, dup := seen[label]; !dup {
				seen[label] = struct{}{}
				sum.SkippedVars = append(sum.SkippedVars, label)
			}
			continue
		}
		kept = append(kept, v)
	}
	st.Vars = kept
	sum.Vars = len(kept)
	sort.Strings(sum.SkippedVars)

	return sum
}
