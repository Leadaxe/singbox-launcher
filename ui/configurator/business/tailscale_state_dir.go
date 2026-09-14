// File tailscale_state_dir.go — каталог состояния tailnet на операциях UI
// (SPEC 122, «Каталог состояния: жизненный цикл», нормы 1–2).
//
// # Почему это здесь, а не в core/config
//
// core/config умеет две вещи: имя каталога по ФИНАЛЬНОМУ тегу и файловые
// операции над корнем. Чего он не знает — это МОДЕЛИ: где лежит узел, папка
// он или корень, какая у контейнера тег-политика. Финальный тег складывается
// из контейнера и узла, и складывать его умеет только тот, у кого на руках
// модель, — то есть эта сторона.
//
// # Почему не «просто дождаться GC на сборке»
//
// GC умеет одно — СНОСИТЬ лишнее. Переименование узла для него неотличимо от
// «старый узел удалили, новый завели»: каталог со старым именем становится
// сиротой и уезжает в удаление вместе с ключом устройства. Пользователь
// переименовал узел — и получил «залогинься заново». Поэтому переезд каталога
// обязан произойти В МОМЕНТ правки, до всякой сборки.
//
// # Чего эти вызовы НЕ ловят
//
// Импорт бэкапа, fetch подписки, ручную правку state.json. Вешать вызов на
// каждый такой путь значило бы ловить их вечно; их добирает GC на ближайшей
// сборке (норма 3) — ценой перелогина, если состав изменился переименованием
// мимо UI.
package business

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// NodeIsTailscale — узел это endpoint tailnet с каталогом состояния ПОД НАШИМ
// КОРНЕМ.
//
// Явный `state_directory` в теле = каталог задал пользователь, он лежит вне
// корня, и трогать его лаунчер не вправе (ни переносить, ни сносить).
func NodeIsTailscale(n *corestate.Node) bool {
	if n == nil || n.Kind != corestate.SourceKindServer || len(n.Body) == 0 {
		return false
	}
	var body map[string]interface{}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return false
	}
	t, _ := body["type"].(string)
	if strings.ToLower(strings.TrimSpace(t)) != config.SchemeTailscale {
		return false
	}
	_, explicit := body["state_directory"]
	return !explicit
}

// tailscaleFinalTag — финальный тег узла: тег-политика контейнера поверх
// сырого тега.
//
// Суффикса уникализации здесь нет и быть не может: его назначает глобальный
// счётчик сборки, которого у правки в UI нет. Перекос осознанный и в сторону
// СОХРАНЕНИЯ — см. CollectTailscaleStateDirNames: лишнее имя оставляет
// каталог жить, недостающее сносит. Сама формула — corestate.TagPolicy.FinalTag.
func tailscaleFinalTag(container *wizardmodels.Source, rawTag string) string {
	rawTag = strings.TrimSpace(rawTag)
	if container == nil {
		return rawTag
	}
	return container.TagPolicy.FinalTag(rawTag)
}

// SourceByID — источник по ULID. Экспортная обёртка над внутренним поиском:
// вызовам жизненного цикла каталога нужен КОНТЕЙНЕР узла (за его
// тег-политикой), а окно источника держит на руках только ULID, снятый при
// открытии, — индекс к моменту Save мог уехать.
func SourceByID(m *wizardmodels.WizardModel, id string) *wizardmodels.Source {
	return findSourceByID(m, strings.TrimSpace(id))
}

// tailscaleFinalTagOfSource — финальный тег узла, лежащего в этом источнике.
//
// Источник может быть и КОРНЕВЫМ УЗЛОМ (Node встроен в Source): у него
// тег-политики нет по построению, и финальный тег равен сырому. Одна функция
// на оба случая нужна затем, что перенос узла видит контейнер-источник
// одинаково — как *Source, — и разводить «папка» с «корневым узлом» по месту
// вызова значило бы повторить развилку у каждого из четырёх переносов.
func tailscaleFinalTagOfSource(src *wizardmodels.Source, rawTag string) string {
	if src != nil && sourceKindIsNode(src.Kind) {
		return strings.TrimSpace(rawTag)
	}
	return tailscaleFinalTag(src, rawTag)
}

// renameTailscaleStateDirTo — переезд каталога в папку с известным ULID.
//
// Папка ищется по ULID, а не по индексу: перенос уже подвинул m.Sources
// (removeNodeFromSource мог убрать целый элемент), и индекс, снятый до
// операции, после неё адресует соседа.
func renameTailscaleStateDirTo(
	m *wizardmodels.WizardModel, oldFinalTag, dstFolderID string, moved *corestate.Node,
) {
	if !NodeIsTailscale(moved) {
		return
	}
	var dst *wizardmodels.Source
	if id := strings.TrimSpace(dstFolderID); id != "" && m != nil {
		for i := range m.Sources {
			if strings.TrimSpace(m.Sources[i].ID) == id {
				dst = &m.Sources[i]
				break
			}
		}
	}
	RenameTailscaleStateDirForNode(nil, oldFinalTag, dst, moved)
}

// RemoveTailscaleStateDirForNode — норма 1: узел удалили, каталог уходит
// следом.
//
// `container` = nil для КОРНЕВОГО узла: у корня тег-политики нет вовсе, и
// финальный тег равен сырому.
func RemoveTailscaleStateDirForNode(container *wizardmodels.Source, n *corestate.Node) {
	if !NodeIsTailscale(n) {
		return
	}
	config.RemoveTailscaleStateDir(config.TailscaleStateDirName(tailscaleFinalTag(container, n.Tag)))
}

// RemoveTailscaleStateDirsForSource — норма 1 для источника целиком: папку
// или подписку удалили ВМЕСТЕ с узлами.
//
// Берёт и сам источник (у корневого узла Node встроен в Source), и его
// состав: у папки удаляются каталоги всех её tailnet-узлов.
func RemoveTailscaleStateDirsForSource(src *wizardmodels.Source) {
	if src == nil {
		return
	}
	if sourceKindIsNode(src.Kind) {
		// Корневой узел: политики нет, финальный тег = сырой.
		RemoveTailscaleStateDirForNode(nil, &src.Node)
		return
	}
	for i := range src.Nodes {
		RemoveTailscaleStateDirForNode(src, &src.Nodes[i])
	}
}

// RenameTailscaleStateDirForNode — норма 2: финальный тег узла сменился,
// каталог переезжает вместе с ним.
//
// Зовётся ТРЕМЯ видами правки, и все три — смена финального тега:
// переименование узла (сырой тег), перенос между контейнерами (сменилась
// политика) и смена тег-политики контейнера (сменилась приставка у всех его
// узлов сразу).
func RenameTailscaleStateDirForNode(
	oldContainer *wizardmodels.Source, oldRawTag string,
	newContainer *wizardmodels.Source, n *corestate.Node,
) {
	if !NodeIsTailscale(n) {
		return
	}
	oldName := config.TailscaleStateDirName(tailscaleFinalTag(oldContainer, oldRawTag))
	newName := config.TailscaleStateDirName(tailscaleFinalTag(newContainer, n.Tag))
	config.RenameTailscaleStateDir(oldName, newName)
}

// RenameTailscaleStateDirsForTagPolicy — норма 2 для СМЕНЫ ТЕГ-ПОЛИТИКИ
// контейнера: приставка меняется разом у всех его узлов, значит и каталоги
// переезжают все разом.
//
// `oldPolicy` — политика ДО правки (nil = её не было). Состав берётся из
// `live`: это живая запись модели, а не снимок окна, — узлы, родившиеся
// фоновым fetch'ем, тоже носят каталоги.
func RenameTailscaleStateDirsForTagPolicy(
	live *wizardmodels.Source, oldPolicy *corestate.TagPolicy, newPolicy *corestate.TagPolicy,
) {
	if live == nil {
		return
	}
	oldP, newP := tagPolicyOrZero(oldPolicy), tagPolicyOrZero(newPolicy)
	if oldP == newP {
		return
	}
	for i := range live.Nodes {
		n := &live.Nodes[i]
		if !NodeIsTailscale(n) {
			continue
		}
		raw := strings.TrimSpace(n.Tag)
		config.RenameTailscaleStateDir(
			config.TailscaleStateDirName(oldP.FinalTag(raw)),
			config.TailscaleStateDirName(newP.FinalTag(raw)),
		)
	}
}

func tagPolicyOrZero(p *corestate.TagPolicy) corestate.TagPolicy {
	if p == nil {
		return corestate.TagPolicy{}
	}
	return *p
}
