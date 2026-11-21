package template

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Контейнер объявлений переменных шаблона (SPEC 183, TEMPLATE_LANG §2.3).
//
// Форм две, в одном шаблоне — ровно одна:
//
//	"sections": [{"id": "tun", "name": "TUN", "description": "…", "vars": [ … ]}, …]
//	"vars":     [ … ]                       плоская: фрагменты корпуса и шаблоны
//	                                        лаунчера, выпущенные до SPEC 183
//
// Разделы разворачиваются в плоский список в порядке «разделы по порядку,
// внутри раздела переменные по порядку» — тот же порядок, по которому идёт
// однопроходное разрешение умолчаний. Весь остальной код работает с плоским
// списком; форму контейнера знает только этот файл.

// DefaultSectionChapter — `chapter` раздела, если поле не задано.
const DefaultSectionChapter = "core"

// TemplateSection — раздел объявлений: заголовок группы строк Settings.
type TemplateSection struct {
	// ID — стабильный машинный идентификатор, kebab-case, уникален в шаблоне.
	ID string
	// Name — заголовок раздела в интерфейсе.
	Name string
	// Description — пояснение под заголовком; пусто — строки нет.
	Description string
	// Chapter — экран-владелец раздела у стороны с несколькими экранами.
	// Лаунчер поле читает, но не использует: вкладка Settings одна.
	Chapter string
}

var sectionIDRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// DecodeVarContainerRaw читает контейнер объявлений из сырого JSON шаблона.
//
// items — объявления в плоском порядке, сырыми (читателю, которому нужны не
// все поля TemplateVar); sectionOf[i] — id раздела items[i], пусто у плоской
// формы; sections — разделы по порядку, nil у плоской формы.
func DecodeVarContainerRaw(raw []byte) (items []json.RawMessage, sectionOf []string, sections []TemplateSection, err error) {
	var root struct {
		Sections *[]json.RawMessage `json:"sections"`
		Vars     *[]json.RawMessage `json:"vars"`
	}
	if err := json.Unmarshal(stripUTF8BOM(raw), &root); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if root.Sections != nil && root.Vars != nil {
		return nil, nil, nil, fmt.Errorf("template sets both sections and vars: variables are declared in one container, sections[] or flat vars[]")
	}
	if root.Sections == nil {
		if root.Vars == nil {
			return nil, nil, nil, nil
		}
		items = *root.Vars
		return items, make([]string, len(items)), nil, nil
	}

	seen := map[string]struct{}{}
	for i, sRaw := range *root.Sections {
		var s struct {
			ID          string             `json:"id"`
			Name        string             `json:"name"`
			Description string             `json:"description"`
			Chapter     string             `json:"chapter"`
			Vars        *[]json.RawMessage `json:"vars"`
		}
		if err := json.Unmarshal(sRaw, &s); err != nil {
			return nil, nil, nil, fmt.Errorf("sections[%d]: %w", i, err)
		}
		id := strings.TrimSpace(s.ID)
		if id == "" {
			return nil, nil, nil, fmt.Errorf("sections[%d]: empty id", i)
		}
		if !sectionIDRE.MatchString(id) {
			return nil, nil, nil, fmt.Errorf("sections[%d]: invalid id %q (expected kebab-case: lowercase letters, digits, single dashes)", i, id)
		}
		if _, dup := seen[id]; dup {
			return nil, nil, nil, fmt.Errorf("sections: duplicate id %q", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(s.Name) == "" {
			return nil, nil, nil, fmt.Errorf("sections[%s]: empty name", id)
		}
		if s.Vars == nil {
			return nil, nil, nil, fmt.Errorf("sections[%s]: vars is required", id)
		}
		chapter := strings.TrimSpace(s.Chapter)
		if chapter == "" {
			chapter = DefaultSectionChapter
		}
		sections = append(sections, TemplateSection{
			ID:          id,
			Name:        strings.TrimSpace(s.Name),
			Description: strings.TrimSpace(s.Description),
			Chapter:     chapter,
		})
		for _, vRaw := range *s.Vars {
			items = append(items, vRaw)
			sectionOf = append(sectionOf, id)
		}
	}
	return items, sectionOf, sections, nil
}

// DecodeVarContainer — объявления шаблона плоским списком и его разделы.
// У переменной из раздела заполнено TemplateVar.Section. Запись-разделитель
// внутри раздела — ошибка: границу группы задаёт сам раздел.
func DecodeVarContainer(raw []byte) ([]TemplateVar, []TemplateSection, error) {
	items, sectionOf, sections, err := DecodeVarContainerRaw(raw)
	if err != nil {
		return nil, nil, err
	}
	vars := make([]TemplateVar, 0, len(items))
	for i, vRaw := range items {
		var v TemplateVar
		if err := json.Unmarshal(vRaw, &v); err != nil {
			if sectionOf[i] != "" {
				return nil, nil, fmt.Errorf("sections[%s].vars: %w", sectionOf[i], err)
			}
			return nil, nil, fmt.Errorf("vars[%d]: %w", i, err)
		}
		v.Section = sectionOf[i]
		if v.Separator && v.Section != "" {
			return nil, nil, fmt.Errorf("sections[%s].vars: separator is not allowed inside a section, the section itself is the group", v.Section)
		}
		vars = append(vars, v)
	}
	return vars, sections, nil
}

// varCtx — адрес объявления в тексте ошибки загрузки: у переменной из раздела
// `sections[<id>].vars[<name>]`, у плоской формы прежний `vars[<i>]`.
func varCtx(i int, v TemplateVar) string {
	if v.Section == "" {
		return fmt.Sprintf("vars[%d]", i)
	}
	return fmt.Sprintf("sections[%s].vars[%s]", v.Section, strings.TrimSpace(v.Name))
}
