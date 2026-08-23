// File source_chain_rewrite.go — переопределения опций звена (SPEC 110).
//
// `rewrite` — JSON merge-patch (RFC 7396) поверх опций узла, ключ верхнего
// уровня — тип outbound'а. Правится тремя полями: тип, параметр, значение.
//
// Значение вводится КАК JSON, а не как текст. Иначе неразличимы вещи,
// которые меняют смысл патча: `null` удаляет ключ, `"null"` записывает
// строку; `1280` — число, `"1280"` — строка, и второе ядро отвергает.
// Проверено на ядре 1.14.0-lx.27-rc.5: `packet_encoding: null` снимает
// ключ и узел стартует, а `packet_encoding: "мусор"` валит ЗАПУСК — не
// `check`. Значит форма обязана проверять сама.
//
// Вложенность выражается самим значением (`{"utls":{"fingerprint":"safari"}}`),
// а не путём с точками: так в поле лежит ровно то, что уедет в конфиг.
package tabs

import (
	"encoding/json"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
)

// rewriteRow — одна строка редактора: «тип · параметр · значение».
type rewriteRow struct {
	Type  string
	Key   string
	Value string // сырой JSON, как ввёл пользователь
}

// rewriteEditor — таблица переопределений.
type rewriteEditor struct {
	rows     []rewriteRow
	box      *fyne.Container
	types    []string // типы узлов, реально стоящих в цепочке
	onChange func()
}

func newRewriteEditor(onChange func()) *rewriteEditor {
	return &rewriteEditor{box: container.NewVBox(), onChange: onChange}
}

// Load разворачивает map ядра в строки таблицы.
//
// Порядок строк не значим (это map), поэтому сортируем — иначе таблица
// перетасовывалась бы при каждом открытии.
func (e *rewriteEditor) Load(rewrite map[string]interface{}) {
	e.rows = nil
	for typeName, patch := range rewrite {
		obj, ok := patch.(map[string]interface{})
		if !ok {
			// Ядро требует объект (`rewrite[type]: must be a JSON object`).
			// Чужую форму не теряем молча — показываем строкой, которую
			// пользователь увидит и поправит.
			raw, _ := json.Marshal(patch)
			e.rows = append(e.rows, rewriteRow{Type: typeName, Value: string(raw)})
			continue
		}
		for key, val := range obj {
			raw, err := json.Marshal(val)
			if err != nil {
				continue
			}
			e.rows = append(e.rows, rewriteRow{Type: typeName, Key: key, Value: string(raw)})
		}
	}
	sort.SliceStable(e.rows, func(i, j int) bool {
		if e.rows[i].Type != e.rows[j].Type {
			return e.rows[i].Type < e.rows[j].Type
		}
		return e.rows[i].Key < e.rows[j].Key
	})
	e.rebuild()
}

// Collect сворачивает строки обратно в map ядра.
//
// Строки с ошибкой ввода пропускаются — они уже подсвечены в форме, и
// записать «примерно то, что имелось в виду» нельзя: ядро на таком не
// стартует.
func (e *rewriteEditor) Collect() map[string]interface{} {
	out := make(map[string]interface{}, len(e.rows))
	for _, r := range e.rows {
		t := strings.TrimSpace(r.Type)
		k := strings.TrimSpace(r.Key)
		if t == "" || k == "" {
			continue
		}
		var val interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(r.Value)), &val); err != nil {
			continue
		}
		obj, _ := out[t].(map[string]interface{})
		if obj == nil {
			obj = make(map[string]interface{}, 2)
			out[t] = obj
		}
		obj[k] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SetTypes задаёт список типов для выпадающего списка.
func (e *rewriteEditor) SetTypes(types []string) {
	e.types = types
	e.rebuild()
}

// rowError — что не так со строкой. Пусто, если всё в порядке.
//
// Проверяется здесь, а не при сохранении: ошибку надо видеть в тот момент,
// когда её делаешь.
func (e *rewriteEditor) rowError(idx int) string {
	r := e.rows[idx]
	t := strings.TrimSpace(r.Type)
	k := strings.TrimSpace(r.Key)
	v := strings.TrimSpace(r.Value)
	if t == "" || k == "" || v == "" {
		return "" // строка ещё заполняется — не ругаемся раньше времени
	}
	if !json.Valid([]byte(v)) {
		return locale.T("wizard.chain.rewrite_err_json")
	}
	// Дубль «тип + параметр» молча перебил бы предыдущую строку: в map
	// остался бы только последний, и пользователь не узнал бы, какой.
	for i, other := range e.rows {
		if i == idx {
			continue
		}
		if strings.TrimSpace(other.Type) == t && strings.TrimSpace(other.Key) == k {
			return locale.Tf("wizard.chain.rewrite_err_dup", t, k)
		}
	}
	return ""
}

func (e *rewriteEditor) rebuild() {
	if e.box == nil {
		return
	}
	e.box.Objects = e.box.Objects[:0]

	for i := range e.rows {
		idx := i

		typeEntry := widget.NewSelectEntry(e.types)
		typeEntry.SetText(e.rows[idx].Type)
		typeEntry.SetPlaceHolder(locale.T("wizard.chain.rewrite_ph_type"))
		typeEntry.OnChanged = func(s string) {
			e.rows[idx].Type = s
			e.changed()
			e.refreshErrors()
		}

		keyEntry := widget.NewEntry()
		keyEntry.SetText(e.rows[idx].Key)
		keyEntry.SetPlaceHolder(locale.T("wizard.chain.rewrite_ph_key"))
		keyEntry.OnChanged = func(s string) {
			e.rows[idx].Key = s
			e.changed()
			e.refreshErrors()
		}

		valEntry := widget.NewEntry()
		valEntry.SetText(e.rows[idx].Value)
		valEntry.SetPlaceHolder(locale.T("wizard.chain.rewrite_ph_value"))
		valEntry.OnChanged = func(s string) {
			e.rows[idx].Value = s
			e.changed()
			e.refreshErrors()
		}

		del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			e.rows = append(e.rows[:idx:idx], e.rows[idx+1:]...)
			e.rebuild()
			e.changed()
		})
		del.Importance = widget.LowImportance

		// Тип и параметр — узкие фиксированной ширины, значение тянется:
		// именно оно бывает длинным (вложенный объект).
		const narrow = 140
		head := container.NewGridWithColumns(2, typeEntry, keyEntry)
		headWrap := container.NewGridWrap(fyne.NewSize(narrow*2, typeEntry.MinSize().Height), head)
		row := container.NewBorder(nil, nil, headWrap, del, valEntry)

		e.box.Add(row)

		if msg := e.rowError(idx); msg != "" {
			warn := widget.NewLabel("⚠️ " + msg)
			warn.Importance = widget.WarningImportance
			warn.Wrapping = fyne.TextWrapWord
			e.box.Add(warn)
		}
	}

	addBtn := widget.NewButtonWithIcon(locale.T("wizard.chain.rewrite_add"), theme.ContentAddIcon(), func() {
		e.rows = append(e.rows, rewriteRow{})
		e.rebuild()
	})
	addBtn.Importance = widget.LowImportance
	e.box.Add(container.NewCenter(addBtn))
	e.box.Refresh()
}

// refreshErrors перерисовывает только подписи об ошибках.
//
// Через полную пересборку: строк единицы, а точечное обновление потребовало
// бы держать ссылки на подписи и следить, что они не разъехались с рядами.
func (e *rewriteEditor) refreshErrors() { e.rebuild() }

func (e *rewriteEditor) changed() {
	if e.onChange != nil {
		e.onChange()
	}
}

// Content — таблица с шапкой и подсказками.
func (e *rewriteEditor) Content() fyne.CanvasObject {
	head := widget.NewLabelWithStyle(locale.T("wizard.chain.rewrite_head"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Патч применяется к позициям со ВТОРОЙ: первая идёт в сеть как есть
	// (`protocol/chain/transform.go:131`). Без этой строки правило «для
	// vless» читалось бы как действующее на все vless-узлы цепочки.
	scope := widget.NewLabel(locale.T("wizard.chain.rewrite_scope"))
	scope.Wrapping = fyne.TextWrapWord
	scope.Importance = widget.LowImportance

	// null — не значение, а удаление ключа (RFC 7396, проверено на ядре).
	// Единственное, что нельзя вывести из формы самой.
	hint := widget.NewLabel(locale.T("wizard.chain.rewrite_hint"))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	return container.NewVBox(head, scope, hint, e.box)
}
