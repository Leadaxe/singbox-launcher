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

	// Тег узла был уникален среди серверов? Только тогда tag-only ссылку можно
	// уверенно приписать этому узлу.
	//
	// Считаем ДО перезаписи, то есть по состоянию «переименованный источник ещё
	// носит nodeTag»: сам он в подсчёт не входит, а любой ДРУГОЙ носитель того
	// же имени делает тег неоднозначным (SPEC 113-E). Раньше подсчёт шёл по уже
	// изменённой модели — переименованный источник тег сменил, тёзка оставался
	// один, тег объявлялся уникальным, и сброс гасил ЧУЖИЕ tag-only ссылки.
	tagIsUnique := nodeTag != ""
	if tagIsUnique {
		for i := range m.Sources {
			s := &m.Sources[i]
			if s.Kind != wizardmodels.SourceKindServer && s.Kind != wizardmodels.SourceKindChain {
				continue
			}
			// Переименованный источник — это и есть прежний носитель имени;
			// сравнивать его текущий (уже новый) тег бессмысленно.
			if sourceID != "" && strings.TrimSpace(s.ID) == sourceID {
				continue
			}
			if strings.TrimSpace(s.NodeTagOrLabel()) == nodeTag {
				tagIsUnique = false
				break
			}
		}
	}

	var affected []string
	for i := range m.Sources {
		s := &m.Sources[i]
		link := s.Detour
		if link == nil {
			continue
		}
		refID := strings.TrimSpace(link.FolderID)
		refTag := strings.TrimSpace(link.Tag)
		if refID == "" && refTag == "" {
			continue
		}

		hit := false
		switch {
		case refID != "" && sourceID != "":
			// Ссылка на узел ПАПКИ: сброс нужен только если она ведёт именно
			// сюда и именно на прежнее имя. Ссылка на другой узел той же
			// папки переименования не заметила.
			hit = refID == sourceID && (nodeTag == "" || refTag == nodeTag)
		case refID == "" && tagIsUnique:
			// Ссылка корневого пространства: она указывает на этот узел, раз
			// имя было его и ничьим больше.
			hit = refTag == nodeTag
		}
		if !hit {
			continue
		}

		s.Detour = nil
		affected = append(affected, SourceDisplayName(*s))
	}
	return affected
}

// SourceDisplayName — как источник зовут пользователю: подпись, за ней тег
// узла, за ним URL/URI. Тот же порядок, что у диагностики сборки
// (config.sourceDisplayName), чтобы имя в окне и имя в логе совпадали.
func SourceDisplayName(s wizardmodels.Source) string {
	// SPEC 116 W4: у КОНТЕЙНЕРА имя канонически живёт в Name (Label —
	// отображаемое имя узловых kind'ов, sources_v7.go:181). Порядок здесь
	// обязан совпадать с corestate.displayName(), иначе папка со старевшим
	// Label звалась бы в диалогах одним именем, а в списке — другим.
	if s.Kind == wizardmodels.SourceKindFolder || s.Kind == wizardmodels.SourceKindSubscription {
		if v := strings.TrimSpace(s.Name); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(s.Label); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.Name); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.Tag); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.URL); v != "" {
		return v
	}
	if s.Origin != nil {
		if v := strings.TrimSpace(s.Origin.Raw); v != "" {
			return v
		}
	}
	return s.ID
}
