// File tag_guard_model.go — единый гард занятости тегов на стороне модели
// (SPEC 118 W4, §4.B.10; features/directions.md §8-9).
//
// Гард сборки (core/config/tag_guard.go) работает по СБОРОЧНОЙ форме и знает
// её теги. Операциям именования и сбросу осиротевших целей правил нужен тот
// же ответ, но по МОДЕЛИ визарда — и это ровно те же виды тегов:
//
//   - Направления и их твины `<tag>-auto`;
//   - replace-теги свёрнутых папок и их `-auto`-двойники (режим both);
//   - теги верхних узлов (server / chain / auto вне папок);
//   - системные теги шаблона и активных пресетов.
//
// Почему это критично для reset: `resetForeignRuleTargets` заменяет на direct
// цель правила, которой нет среди известных. Не знай он про replace-теги — и
// первая же загрузка мигрированного состояния сбросила бы живое правило на
// `[P]select` в direct (deps-К2). Правило блока: новый вид тега — сначала
// сюда, потом в модель.
package business

import (
	"strings"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// ModelTagOwners — все занятые теги модели с указанием вида владельца.
//
// Ключ — тег, значение — человекочитаемый вид (для сообщения об отказе).
func ModelTagOwners(model *wizardmodels.WizardModel) map[string]string {
	owners := make(map[string]string, 32)
	if model == nil {
		return owners
	}
	claim := func(tag, kind string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, taken := owners[tag]; taken {
			return // первый владелец побеждает: ответ обязан быть детерминированным
		}
		owners[tag] = kind
	}

	for i := range model.GlobalOutbounds {
		d := &model.GlobalOutbounds[i]
		claim(d.Tag, "Direction")
		if d.Auto != nil && d.Tag != "" {
			claim(d.AutoTag(), "Direction auto group")
		}
	}
	for _, tag := range ModelReplaceTags(model) {
		claim(tag, "folder replacement")
	}
	for _, tag := range ModelRootNodeTags(model) {
		claim(tag, "node")
	}
	for _, tag := range GetAvailableOutbounds(model) {
		claim(tag, "template system tag")
	}
	return owners
}

// ModelReplaceTags — теги замен всех свёрнутых папок модели, включая
// `-auto`-двойники режима both.
//
// Формула двойника здесь та же, что у сборки и у твинов Направлений: на её
// совпадении держится узнавание пар по тегу.
func ModelReplaceTags(model *wizardmodels.WizardModel) []string {
	if model == nil {
		return nil
	}
	var out []string
	for i := range model.Sources {
		r := model.Sources[i].Replace
		if r == nil || strings.TrimSpace(r.Tag) == "" {
			continue
		}
		out = append(out, r.Tag)
		if r.Mode == corestate.FolderReplaceBoth {
			out = append(out, r.Tag+"-auto")
		}
	}
	return out
}

// ModelRootNodeTags — теги ВЕРХНИХ узлов (вне папок): server, chain, auto в
// корне `sources[]`.
//
// Узлы папок сюда НЕ входят: их финальный тег — производная тег-политики
// папки, он вычисляется на сборке и целью правила не предлагается (путь
// трафика к узлу всегда лежит через выбор — features/directions.md §1).
func ModelRootNodeTags(model *wizardmodels.WizardModel) []string {
	if model == nil {
		return nil
	}
	var out []string
	for i := range model.Sources {
		s := &model.Sources[i]
		switch s.Kind {
		case corestate.SourceKindServer, corestate.SourceKindChain, corestate.SourceKindAuto:
			if tag := strings.TrimSpace(s.NodeTagOrLabel()); tag != "" {
				out = append(out, tag)
			}
		}
	}
	return out
}

// KnownRuleTargetTags — множество известных целей для сброса осиротевших
// правил (§4.B.10): всё из гарда плюс выключенные Направления.
//
// Выключенное Направление — не «чужая цель», а временно снятая своя: сброс
// необратим, а правило на отсутствующий в конфиге тег и так чистится на
// сборке.
func KnownRuleTargetTags(model *wizardmodels.WizardModel) map[string]bool {
	known := make(map[string]bool, 48)
	for tag := range ModelTagOwners(model) {
		known[tag] = true
	}
	for _, tag := range AllDirectionTags(model) {
		known[tag] = true
	}
	return known
}
