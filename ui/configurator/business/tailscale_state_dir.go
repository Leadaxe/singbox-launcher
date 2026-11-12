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
// GC умеет одно — СНОСИТЬ лишнее. Переименование узла для него выглядит как
// «старый узел удалили, новый завели». Поэтому в момент правки узел без
// `state_dir_id` получает имя текущего каталога (SPEC 179): каталог не двигается,
// устройство tailnet остаётся тем же.
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

// TailscaleNodeFinalTag — финальный тег узла контейнера с этим ULID (пусто —
// корневой узел): им узел назван в статусе работающего ядра.
func TailscaleNodeFinalTag(m *wizardmodels.WizardModel, containerID, rawTag string) string {
	return tailscaleFinalTag(SourceByID(m, containerID), rawTag)
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

// freezeTailscaleStateDirTo — узел переехал в папку с известным ULID.
//
// Папка ищется по ULID, а не по индексу: перенос уже подвинул m.Sources
// (removeNodeFromSource мог убрать целый элемент), и индекс, снятый до
// операции, после неё адресует соседа.
func freezeTailscaleStateDirTo(
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
	FreezeTailscaleStateDirForNode(nil, oldFinalTag, dst, moved)
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
	config.RemoveTailscaleStateDir(config.TailscaleStateDirNameFor(n.StateDirID, tailscaleFinalTag(container, n.Tag)))
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

// reseedTailscaleStateDir — узел появился в модели заново (создан, вставлен,
// скопирован): свой каталог, тот же каталог двум узлам нельзя.
func reseedTailscaleStateDir(n *corestate.Node) {
	if n == nil {
		return
	}
	n.StateDirID = ""
	config.SeedTailscaleStateDir(n)
}

// FreezeTailscaleStateDirForNode — норма 2: финальный тег узла меняется, каталог
// остаётся на месте. Узел без `state_dir_id` получает имя каталога по старому
// тегу; узел с параметром ничего не замечает.
//
// Зовётся ТРЕМЯ видами правки, и все три — смена финального тега: переименование
// узла (сырой тег), перенос между контейнерами (другая политика) и смена
// тег-политики контейнера.
func FreezeTailscaleStateDirForNode(
	oldContainer *wizardmodels.Source, oldRawTag string,
	newContainer *wizardmodels.Source, n *corestate.Node,
) {
	if !NodeIsTailscale(n) || n.StateDirID != "" {
		return
	}
	oldName := config.TailscaleStateDirName(tailscaleFinalTag(oldContainer, oldRawTag))
	newName := config.TailscaleStateDirName(tailscaleFinalTag(newContainer, n.Tag))
	if oldName != newName {
		n.StateDirID = oldName
	}
}

// FreezeTailscaleStateDirsForTagPolicy — норма 2 для СМЕНЫ ТЕГ-ПОЛИТИКИ
// контейнера: приставка меняется разом у всех его узлов.
//
// `oldPolicy` — политика ДО правки (nil = её не было). Состав берётся из `live`:
// это живая запись модели. У подписки записи узла нет: её узлам каталог остаётся
// по тегу, новая приставка заведёт новое устройство.
func FreezeTailscaleStateDirsForTagPolicy(
	live *wizardmodels.Source, oldPolicy *corestate.TagPolicy, newPolicy *corestate.TagPolicy,
) {
	if live == nil || live.Kind == corestate.SourceKindSubscription {
		return
	}
	oldP, newP := tagPolicyOrZero(oldPolicy), tagPolicyOrZero(newPolicy)
	if oldP == newP {
		return
	}
	for i := range live.Nodes {
		n := &live.Nodes[i]
		if !NodeIsTailscale(n) || n.StateDirID != "" {
			continue
		}
		raw := strings.TrimSpace(n.Tag)
		oldName := config.TailscaleStateDirName(oldP.FinalTag(raw))
		if oldName != config.TailscaleStateDirName(newP.FinalTag(raw)) {
			n.StateDirID = oldName
		}
	}
}

func tagPolicyOrZero(p *corestate.TagPolicy) corestate.TagPolicy {
	if p == nil {
		return corestate.TagPolicy{}
	}
	return *p
}
