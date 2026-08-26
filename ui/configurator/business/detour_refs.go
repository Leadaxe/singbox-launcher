package business

import (
	"strings"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 112-A, «Смена идентичности узла = сброс ссылок с предупреждением».
//
// Резолв ссылки на узел при сборке СТРОГИЙ: она жива, только если сошлись обе
// части — источник по ULID и узел по identity-тегу внутри него. Расхождение
// лечить молча нельзя: узел под другим именем — другой узел, и подставить его
// значило бы пустить трафик источника через хоп, которого пользователь не
// выбирал.
//
// Из-за этого честность обязан обеспечивать UI В МОМЕНТ операции: правка,
// меняющая идентичность узла (сегодня это переименование node_tag
// ручного сервера), находит ВСЕ ссылки на него, сбрасывает их в «нет detour»
// и сообщает пользователю, каких источников это коснулось. Иначе следующая
// сборка выбросила бы ссылающиеся источники fail-closed, а пользователь узнал
// бы об этом из лога.
//
// Сброс применяется вместе с сохранением формы, поэтому окно информирующее:
// отменять в нём уже нечего.

// ResetDetourNodeRefs сбрасывает все ссылки на узел источника sourceID и
// возвращает имена источников, которых это коснулось (для окна-предупреждения).
//
// nodeTag — identity-тег узла ДО правки: именно на него смотрят живые ссылки.
// Пустой nodeTag означает «сбросить любую ссылку на этот источник».
//
// Ссылка без source_id (переходная форма) считается указывающей на этот узел,
// когда её тег совпадает с прежним И другого источника с таким же тегом узла
// нет: иначе сброс задел бы чужую ссылку.
func ResetDetourNodeRefs(m *wizardmodels.WizardModel, sourceID, nodeTag string) []string {
	if m == nil {
		return nil
	}
	sourceID = strings.TrimSpace(sourceID)
	nodeTag = strings.TrimSpace(nodeTag)
	if sourceID == "" && nodeTag == "" {
		return nil
	}

	// Тег узла уникален среди серверов? Только тогда tag-only ссылку можно
	// уверенно приписать этому узлу.
	tagIsUnique := nodeTag != ""
	if tagIsUnique {
		seen := 0
		for i := range m.Sources {
			s := &m.Sources[i]
			if s.Type != wizardmodels.SourceTypeServer && s.Type != wizardmodels.SourceTypeChain {
				continue
			}
			if strings.TrimSpace(s.NodeTagOrLabel()) == nodeTag {
				seen++
			}
		}
		tagIsUnique = seen <= 1
	}

	var affected []string
	for i := range m.Sources {
		s := &m.Sources[i]
		refID := strings.TrimSpace(s.DetourNodeSourceID)
		refTag := strings.TrimSpace(s.DetourNodeTag)
		if refID == "" && refTag == "" && strings.TrimSpace(s.DetourNodeHash) == "" {
			continue
		}

		hit := false
		switch {
		case refID != "" && sourceID != "":
			// Полная ссылка: сброс нужен только если она ведёт именно сюда и
			// именно на прежнее имя. Ссылка на другой узел того же источника
			// (источник-подписка) переименования не заметила.
			hit = refID == sourceID && (nodeTag == "" || refTag == nodeTag)
		case refID == "" && tagIsUnique:
			// Переходная ссылка по финальному тегу: она указывает на этот узел,
			// раз имя было его и ничьим больше.
			hit = refTag == nodeTag
		}
		if !hit {
			continue
		}

		s.DetourNodeSourceID = ""
		s.DetourNodeTag = ""
		s.DetourNodeLabel = ""
		// Упразднённый хеш гасится заодно: иначе миграция на сборке воскресила
		// бы ссылку, которую пользователю только что показали сброшенной.
		s.DetourNodeHash = ""
		affected = append(affected, SourceDisplayName(*s))
	}
	return affected
}

// SourceDisplayName — как источник зовут пользователю: подпись, за ней тег
// узла, за ним URL/URI. Тот же порядок, что у диагностики сборки
// (config.sourceDisplayName), чтобы имя в окне и имя в логе совпадали.
func SourceDisplayName(s wizardmodels.Source) string {
	if v := strings.TrimSpace(s.Label); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.NodeTag); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.URL); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.URI); v != "" {
		return v
	}
	return s.ID
}
