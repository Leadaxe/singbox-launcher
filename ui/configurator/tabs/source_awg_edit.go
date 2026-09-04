// File source_awg_edit.go — обфускация AmneziaWG у ГОТОВОГО узла-сервера.
//
// Зачем отдельно от парсера ссылок. Поля AWG узел получает при импорте — из
// wireguard://-ссылки, vpn://-профиля Amnezia или вставленного .conf. Но
// сервер, на который узел ходит, может начать требовать обфускацию уже после
// того, как узел заведён, и переспрашивать у провайдера новую ссылку ради
// пяти чисел незачем: тело узла — обычный sing-box endpoint, и эти поля в нём
// правятся на месте.
//
// Что редактируется и почему именно это. Набор исчерпывающий:
//
//	id  — домен маскировки первого decoy-пакета;
//	ib  — браузер, чей QUIC-отпечаток имитируется;
//	jc  — сколько мусорных датаграмм уходит до рукопожатия;
//	jmin/jmax — их размеры.
//
// Протокол маскировки (`ip`) — константа `quic`: единственный проверенный
// режим, и домен требуется именно им. MTU у AWG-узла правится своим полем
// формы (потолок держит парсер, см. awgMaxMTU).
//
// Чего здесь нет намеренно. h1–h4 при значениях 1/2/3/4 — это и есть
// дефолтные типы сообщений WireGuard: писать их незачем, результат тот же.
// Любое ДРУГОЕ значение h1–h4, равно как и ненулевые s1–s4, сдвигает тип или
// размер реального пакета — сервер перестаёт узнавать рукопожатие и молча его
// дропает. Явные i1–i5 несовместимы с сахаром id/ip/ib: ядро отвергает пару.
// Такие наборы задаются вставкой ссылки или .conf, а не этой формой.
package tabs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// awgBlockNoteText — пояснение под полями блока (ключ = английский текст,
// SPEC 111).
const awgBlockNoteText = "Junk datagrams go out before the handshake, and the first one is disguised as traffic to the domain above. Packet padding (s1-s4) and magic headers (h1-h4) stay at the WireGuard defaults — any other value there and the server stops recognising the handshake. Press the button to write these fields into the node."

// awgFixedIP — протокол первого decoy-пакета. Константа, а не настройка.
const awgFixedIP = "quic"

// Дефолты для узла, у которого обфускации ещё нет.
const (
	awgDefaultJC   = "4"
	awgDefaultJMin = "40"
	awgDefaultJMax = "70"
	awgDefaultIB   = "chrome"
)

// awgBrowsers — значения ib, которые понимает ядро.
var awgBrowsers = []string{"chrome", "firefox", "curl"}

// awgSettings — состояние обфускации узла в виде, пригодном для формы.
type awgSettings struct {
	// Enabled — у узла есть поля обфускации прямо сейчас.
	Enabled bool
	Domain  string
	Browser string
	JC      string
	JMin    string
	JMax    string
}

// defaultAWGSettings — что показать, когда у узла обфускации ещё нет.
// Домен пуст намеренно: подставить сюда какой-либо адрес значило бы выбрать
// за пользователя, куда маскироваться, а выбор этот зависит от его трафика.
func defaultAWGSettings() awgSettings {
	return awgSettings{
		Browser: awgDefaultIB,
		JC:      awgDefaultJC,
		JMin:    awgDefaultJMin,
		JMax:    awgDefaultJMax,
	}
}

// awgEditableNode — у узла есть тело WireGuard-endpoint'а, то есть форму
// обфускации ему показывать осмысленно.
//
// Проверка по ТЕЛУ, а не по происхождению: узел мог приехать из .conf, из
// ссылки или из вставленного JSON — тип в теле один и тот же.
func awgEditableNode(node *wizardmodels.Node) bool {
	if node == nil || len(node.Body) == 0 {
		return false
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return false
	}
	t, _ := ob["type"].(string)
	return t == "wireguard"
}

// readAWGSettings достаёт настройки обфускации из тела узла.
//
// Числа приходят из JSON как float64 (или int64 после парсера ссылки) —
// разбираем оба, иначе форма показала бы пустые поля у настроенного узла.
func readAWGSettings(node *wizardmodels.Node) awgSettings {
	out := defaultAWGSettings()
	if node == nil || len(node.Body) == 0 {
		return out
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return out
	}

	domain, _ := ob["id"].(string)
	if domain != "" {
		out.Domain = domain
		out.Enabled = true
	}
	if browser, _ := ob["ib"].(string); browser != "" {
		out.Browser = browser
	}
	for key, dst := range map[string]*string{"jc": &out.JC, "jmin": &out.JMin, "jmax": &out.JMax} {
		if s, ok := awgNumberToString(ob[key]); ok {
			*dst = s
			out.Enabled = true
		}
	}
	return out
}

// awgNumberToString приводит числовое поле тела к строке для поля ввода.
func awgNumberToString(v interface{}) (string, bool) {
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case int:
		return strconv.Itoa(n), true
	case json.Number:
		return n.String(), true
	}
	return "", false
}

// validateAWGSettings проверяет ввод формы до записи в узел.
//
// Проверять обязательно: ядро на негодном домене отвергает узел целиком, а
// нечисловое значение парсер молча выбросил бы — узел ушёл бы в конфиг без
// обфускации, о которой человек просил.
func validateAWGSettings(s awgSettings) error {
	domain := strings.TrimSpace(s.Domain)
	if domain == "" {
		return fmt.Errorf("%s", locale.T("Masquerade domain required"))
	}
	if err := validateMasqueradeDomain(domain); err != nil {
		return fmt.Errorf("%s: %w", locale.T("Masquerade domain"), err)
	}
	if _, err := awgNumberField(s.JC, "jc", 0, 128); err != nil {
		return err
	}
	jmin, err := awgNumberField(s.JMin, "jmin", 0, 1280)
	if err != nil {
		return err
	}
	jmax, err := awgNumberField(s.JMax, "jmax", 0, 1280)
	if err != nil {
		return err
	}
	if jmin > jmax {
		return fmt.Errorf("%s", locale.T("jmin must not exceed jmax"))
	}
	return nil
}

// awgNumberField разбирает числовое поле обфускации и держит его в разумных
// границах. Верхние границы взяты с запасом от рабочего профиля (4/40/70):
// junk — отдельные датаграммы, сервер их игнорирует, но раздувать их незачем.
func awgNumberField(raw, name string, minVal, maxVal int) (int, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, fmt.Errorf("%s", locale.Tf("%s is required", name))
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minVal || n > maxVal {
		return 0, fmt.Errorf("%s", locale.Tf("%s must be %d..%d", name, minVal, maxVal))
	}
	return n, nil
}

// validateMasqueradeDomain проверяет домен по LDH-правилу, которым его
// принимает ядро: метки из букв, цифр и дефиса, дефис не с краю, хотя бы одна
// точка. Форма — единственное место, где человек увидит причину отказа.
func validateMasqueradeDomain(domain string) error {
	if len(domain) > 253 {
		return fmt.Errorf("%s", locale.T("longer than 253 characters"))
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return fmt.Errorf("%s", locale.T("must contain a dot, e.g. example.com"))
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("%s", locale.T("empty or too long label"))
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("%s", locale.T("label starts or ends with a hyphen"))
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			default:
				return fmt.Errorf("%s", locale.T("only letters, digits and hyphens are allowed"))
			}
		}
	}
	return nil
}

// applyAWGSettings записывает обфускацию в тело узла.
//
// Тело правится КЛЮЧАМИ, а не пересборкой: у узла могут быть поля, которых
// наша форма не знает (reserved, s3/s4 из чужой ссылки), и пересборка молча
// их бы потеряла. Порядок ключей внутри объекта json.Marshal раскладывает по
// алфавиту — тело всё равно перезаписывается целиком, так что сравнивать его
// побайтово с прежним смысла нет.
//
// Ошибка = ОТКАТ: узел остаётся прежним, вызывающий показывает причину.
func applyAWGSettings(node *wizardmodels.Node, s awgSettings) error {
	if node == nil || len(node.Body) == 0 {
		return fmt.Errorf("%s", locale.T("Node has no body to edit"))
	}
	if err := validateAWGSettings(s); err != nil {
		return err
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return err
	}

	jc, _ := strconv.Atoi(strings.TrimSpace(s.JC))
	jmin, _ := strconv.Atoi(strings.TrimSpace(s.JMin))
	jmax, _ := strconv.Atoi(strings.TrimSpace(s.JMax))
	ob["jc"] = jc
	ob["jmin"] = jmin
	ob["jmax"] = jmax
	ob["ip"] = awgFixedIP
	ob["id"] = strings.TrimSpace(s.Domain)
	if browser := strings.TrimSpace(s.Browser); browser != "" {
		ob["ib"] = browser
	}
	// Явные i1–i5 ядро отвергает рядом с сахаром id/ip/ib. Узел мог приехать
	// с ними из чужой ссылки — тогда включение обфускации формой обязано их
	// снять, иначе конфиг просто не соберётся.
	for _, k := range []string{"i1", "i2", "i3", "i4", "i5"} {
		delete(ob, k)
	}

	body, err := json.Marshal(ob)
	if err != nil {
		return err
	}
	node.Body = body
	return nil
}

// clearAWGSettings снимает обфускацию с узла: снятая галочка обязана вернуть
// обычный WireGuard, а не оставить половину полей.
//
// Снимаются и s1–s4 с h1–h4: если они пришли из чужой ссылки, без junk и
// маскировки они не обфускация, а ровно тот набор, который ломает рукопожатие
// с обычным сервером.
func clearAWGSettings(node *wizardmodels.Node) error {
	if node == nil || len(node.Body) == 0 {
		return fmt.Errorf("%s", locale.T("Node has no body to edit"))
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return err
	}
	for _, k := range []string{
		"jc", "jmin", "jmax", "ip", "id", "ib",
		"s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4",
		"i1", "i2", "i3", "i4", "i5",
	} {
		delete(ob, k)
	}
	body, err := json.Marshal(ob)
	if err != nil {
		return err
	}
	node.Body = body
	return nil
}

// awgBlock — блок обфускации в форме узла: виджеты плюс перечитывание из
// узла. Собран отдельным типом, потому что форма окна источника и без него
// длинная, а состояние блока (галочка, четыре поля, кнопка) живёт своей
// жизнью и переиспользуется двумя её местами — перестройкой раскладки и
// обновлением после Regen.
type awgBlock struct {
	check   *widget.Check
	domain  *widget.Entry
	browser *widget.Select
	jc      *widget.Entry
	jmin    *widget.Entry
	jmax    *widget.Entry
	apply   *widget.Button
	note    *widget.Label
	rows    []fyne.CanvasObject
	// content — всё, что блок добавляет в форму, одним объектом.
	content *fyne.Container
	// syncing — идёт программная запись значений; обработчики не считают её
	// правкой пользователя.
	syncing bool
}

// newAWGBlock собирает блок обфускации для узла.
//
// nodeRef отдаёт узел рабочей копии (правки буферизуются до Save, как и всё
// в этом окне), onApplied зовётся после успешной записи в тело — форма по
// нему перерисовывает вкладку JSON и помечает состояние изменённым.
func newAWGBlock(
	nodeRef func() *wizardmodels.Node,
	win fyne.Window,
	onApplied func(),
) *awgBlock {
	b := &awgBlock{}

	b.domain = widget.NewEntry()
	// Плейсхолдер — форма записи, а не готовый адрес: подставлять сюда
	// конкретный домен значило бы выбрать за пользователя, куда маскироваться.
	b.domain.SetPlaceHolder(locale.T("domain to masquerade as, e.g. example.com"))
	b.browser = widget.NewSelect(awgBrowsers, nil)
	b.jc = awgNumEntry()
	b.jmin = awgNumEntry()
	b.jmax = awgNumEntry()

	b.note = widget.NewLabel(locale.T(awgBlockNoteText))
	b.note.Wrapping = fyne.TextWrapWord
	b.note.Importance = widget.LowImportance

	b.apply = widget.NewButton(locale.T("Regen"), func() {
		node := nodeRef()
		if node == nil {
			return
		}
		if err := applyAWGSettings(node, b.values()); err != nil {
			dialog.ShowError(err, win)
			return
		}
		b.load(node)
		if onApplied != nil {
			onApplied()
		}
	})
	b.apply.Importance = widget.HighImportance

	b.check = widget.NewCheck(locale.T("AmneziaWG obfuscation"), func(on bool) {
		if b.syncing {
			return
		}
		node := nodeRef()
		if node == nil {
			return
		}
		if !on {
			// Снятая галочка обязана вернуть обычный WireGuard сразу: держать
			// поля до нажатия кнопки значило бы показывать выключенным то,
			// что в теле ещё включено.
			if err := clearAWGSettings(node); err != nil {
				dialog.ShowError(err, win)
				return
			}
			b.load(node)
			if onApplied != nil {
				onApplied()
			}
			return
		}
		// Включение НЕ пишет в тело: домен пуст, писать нечего. Показываем
		// поля и ждём кнопку — она же и проверит ввод.
		b.setRowsVisible(true)
	})

	// Одна строка на весь блок:
	//
	//	id [apteka.ru        ] ib [chrome ▾] jc [3] jmin [1] jmax [3] [Regen]
	//
	// Тянется только домен — он один переменной длины. Остальное прижато к
	// своему содержимому: числа узкие по своей минимальной ширине (см.
	// awgNumEntry), кнопка и выпадающий список — по подписи. В Border
	// растягивается ровно центр, поэтому домен стоит там, а всё прочее ушло
	// в правый край одной HBox-лентой.
	tail := container.NewHBox(
		widget.NewLabel("ib"), b.browser,
		widget.NewLabel("jc"), awgNumCell(b.jc, awgJCDigits),
		widget.NewLabel("jmin"), awgNumCell(b.jmin, awgJSizeDigits),
		widget.NewLabel("jmax"), awgNumCell(b.jmax, awgJSizeDigits),
		b.apply,
	)
	b.rows = []fyne.CanvasObject{
		container.NewBorder(nil, nil, widget.NewLabel("id"), tail, b.domain),
		b.note,
	}
	b.content = container.NewVBox(append([]fyne.CanvasObject{b.check}, b.rows...)...)
	return b
}

// awgNumEntry — поле под число обфускации. Ширину ему задаёт awgNumCell при
// раскладке: сам Entry своего размера не знает.
func awgNumEntry() *widget.Entry { return widget.NewEntry() }

// awgNumCell — поле числа шириной под digits знаков.
//
// Ширина считается ИЗМЕРЕНИЕМ текста, а не пикселями наугад: при другом
// шрифте или масштабе интерфейса фиксированное число обрезало бы цифры или
// оставляло пустое место. К измеренной строке добавляется внутренний отступ
// поля с обеих сторон — без него текст упирался бы в рамку.
func awgNumCell(e *widget.Entry, digits int) fyne.CanvasObject {
	sample := strings.Repeat("8", digits) // самая широкая цифра в большинстве шрифтов
	textSize := theme.TextSize()
	w := fyne.MeasureText(sample, textSize, fyne.TextStyle{}).Width
	sizer := canvas.NewRectangle(color.Transparent)
	sizer.SetMinSize(fyne.NewSize(w+4*theme.Padding(), 0))
	return container.NewStack(sizer, e)
}

// Разрядность полей: jc — счётчик датаграмм (десятки), jmin/jmax — их размеры
// в байтах (сотни). Поле под свою величину и никак не шире.
const (
	awgJCDigits    = 2
	awgJSizeDigits = 3
)

// values — что сейчас набрано в форме.
func (b *awgBlock) values() awgSettings {
	return awgSettings{
		Enabled: b.check.Checked,
		Domain:  b.domain.Text,
		Browser: b.browser.Selected,
		JC:      b.jc.Text,
		JMin:    b.jmin.Text,
		JMax:    b.jmax.Text,
	}
}

// load перечитывает состояние из узла: открытие окна, успешная запись,
// пересборка тела кнопкой Regen происхождения.
func (b *awgBlock) load(node *wizardmodels.Node) {
	s := readAWGSettings(node)
	b.syncing = true
	b.check.SetChecked(s.Enabled)
	b.domain.SetText(s.Domain)
	b.browser.SetSelected(s.Browser)
	b.jc.SetText(s.JC)
	b.jmin.SetText(s.JMin)
	b.jmax.SetText(s.JMax)
	b.syncing = false
	b.setRowsVisible(s.Enabled)
}

// setRowsVisible прячет поля, пока обфускация выключена: выключенный блок не
// должен занимать место строками, которые ни на что не влияют.
func (b *awgBlock) setRowsVisible(on bool) {
	for _, row := range b.rows {
		if row == nil {
			continue
		}
		if on {
			row.Show()
			continue
		}
		row.Hide()
	}
}
