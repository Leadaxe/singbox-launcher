// File source_tailscale_edit.go — роль узла Tailscale в tailnet у ГОТОВОГО
// узла-сервера (SPEC 157).
//
// Зачем отдельно от конструктора «Add server → Tailscale». Конструктор
// спрашивает роль один раз при создании, а меняется она потом: сегодня узел
// ходит через чужой выход (`exit_node`), завтра его самого делают выходом
// (`advertise_exit_node`), послезавтра выход меняют на другую машину. До
// этого блока единственным путём была правка JSON руками — и правка
// происхождения, на которой узел ломался (origin JSON записывался как
// `uri`, Regen падал на «no scheme»).
//
// Что редактируется. Ровно три поля роли:
//
//	advertise_exit_node        — быть выходом для других машин tailnet;
//	exit_node                  — чей выход использовать (имя или IP машины);
//	exit_node_allow_lan_access — не заворачивать локальную сеть в этот выход.
//
// Две роли исключают друг друга: ядро отказывается стартовать с
// `advertise_exit_node` и непустым `exit_node` разом (protocol/tailscale/
// endpoint.go — «cannot advertise an exit node and use an exit node at the
// same time»); проверка живёт в NewEndpoint, то есть `sing-box check` её не
// ловит — падает только запуск (ловушка chain-check-misses-start-errors).
// Поэтому форма не даёт собрать такую пару: галка «быть выходом» гасит
// строку «пользоваться выходом» и снимает её поля с тела, как и
// конструктор. Связь `exit_node_allow_lan_access` → `exit_node` держит
// реестр (`requires`): без выхода поле не пишется.
//
// Остальные поля узла (ключ, hostname, маршруты, теги ACL) здесь не
// правятся: они задаются при создании или в JSON, и менять их у живого узла
// значит перерегистрировать машину — не то, что делают походя из Settings.
package tabs

import (
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// tailscaleBlockNoteText — пояснение под полями блока (ключ = английский
// текст, SPEC 111). Нормативно в двух вещах: роли исключают друг друга, и
// анонс сам по себе ничего не включает — его одобряют в админке tailnet.
const tailscaleBlockNoteText = "Using an exit node sends this node's traffic through another machine of the tailnet; advertising makes this node the exit for others. The two roles exclude each other. Advertising stays pending until it is approved in the tailnet admin console. The Network tab of the node window lists the exit nodes that are online now."

// tailscaleSingboxType — тип тела ядра у узла Tailscale; по нему блок
// решает, показываться ли. Схему реестра по типу даёт обратная карта
// (NodeSchemeForSingboxType), своего имени схемы форма не держит.
const tailscaleSingboxType = "tailscale"

// Ключи тела, которые пишет форма. Снятие любого из них санитайзером =
// ввод негоден, и тело не пишется.
const (
	tailscaleKeyAdvertiseExit = "advertise_exit_node"
	tailscaleKeyExitNode      = "exit_node"
	tailscaleKeyExitLAN       = "exit_node_allow_lan_access"
)

// tailscaleRoleKeys — те же ключи списком: порядок = порядок проверки
// отказа санитайзера.
var tailscaleRoleKeys = []string{tailscaleKeyAdvertiseExit, tailscaleKeyExitNode, tailscaleKeyExitLAN}

// tailscaleRole — роль узла в tailnet в виде, пригодном для формы.
type tailscaleRole struct {
	AdvertiseExit bool
	ExitNode      string
	ExitLAN       bool
}

// tailscaleSchemeOfBody — схема реестра по типу тела, если это Tailscale.
// Пусто — блок узлу не показывается.
func tailscaleSchemeOfBody(ob map[string]interface{}) string {
	t, _ := ob["type"].(string)
	if t != tailscaleSingboxType {
		return ""
	}
	reg, err := registry.Get()
	if err != nil {
		return ""
	}
	scheme, ok := reg.NodeSchemeForSingboxType(t)
	if !ok {
		return ""
	}
	return scheme
}

// tailscaleEditableNode — у узла тело Tailscale-endpoint'а, то есть блок
// роли ему показывать осмысленно. Проверка по ТЕЛУ, а не по происхождению:
// узел мог приехать из конструктора, из вставленного JSON или из бэкапа —
// тип в теле один и тот же.
func tailscaleEditableNode(node *wizardmodels.Node) bool {
	if node == nil || len(node.Body) == 0 {
		return false
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return false
	}
	return tailscaleSchemeOfBody(ob) != ""
}

// readTailscaleRole достаёт роль из тела узла.
func readTailscaleRole(node *wizardmodels.Node) tailscaleRole {
	var out tailscaleRole
	if node == nil || len(node.Body) == 0 {
		return out
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return out
	}
	out.AdvertiseExit, _ = ob[tailscaleKeyAdvertiseExit].(bool)
	out.ExitNode, _ = ob[tailscaleKeyExitNode].(string)
	out.ExitNode = strings.TrimSpace(out.ExitNode)
	out.ExitLAN, _ = ob[tailscaleKeyExitLAN].(bool)
	return out
}

// applyTailscaleRole записывает роль в тело узла.
//
// Тело правится КЛЮЧАМИ, а не пересборкой: у узла есть поля, которых форма
// не знает (auth_key, advertise_routes, listen_port…), и пересборка молча
// их бы потеряла. Снятое с формы поле УДАЛЯЕТСЯ из тела, а не пишется
// false/"": дефолт ядра тот же, а лишний ключ в теле — лишняя строка в
// конфиге и в JSON-вкладке.
//
// Ошибка = ОТКАТ: узел остаётся прежним, вызывающий показывает причину.
func applyTailscaleRole(node *wizardmodels.Node, r tailscaleRole) error {
	if node == nil || len(node.Body) == 0 {
		return fmt.Errorf("%s", locale.T("Node has no body to edit"))
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return err
	}
	delete(ob, tailscaleKeyAdvertiseExit)
	delete(ob, tailscaleKeyExitNode)
	delete(ob, tailscaleKeyExitLAN)
	if r.AdvertiseExit {
		// Роли взаимоисключающие: на анонсе чужой выход в тело не пишется,
		// даже если строка осталась заполненной с прошлого состояния галки.
		ob[tailscaleKeyAdvertiseExit] = true
	} else if exit := strings.TrimSpace(r.ExitNode); exit != "" {
		ob[tailscaleKeyExitNode] = exit
		if r.ExitLAN {
			ob[tailscaleKeyExitLAN] = true
		}
	}
	return writeTailscaleBody(node, ob)
}

// writeTailscaleBody — единственная запись тела формой.
//
// Тело проходит `nodeflow.Sanitize` → `nodeflow.Emit`, как на всех
// остальных входах, и коды узла пересчитываются по тому же реестру.
// Вердикт уровня узла (`Drop`) — ОТКАТ; снятый санитайзером ключ формы —
// отказ с текстом кода (awgRejectedInput — общий с блоком обфускации).
//
// У узла с источником-телом (вид `json`) источник правится вместе с телом:
// иначе «Regen from raw» вернул бы прежнюю роль, а подпись «origin above
// is ignored» врала бы о том, что в источнике лежит. Так же поступает
// Save choice вкладки Network (core.WriteTailscaleExitNodeChoice). Узел из
// ссылки или INI у Tailscale не бывает (URI-формы у схемы нет), но если
// происхождение всё же иного вида — оно не трогается, как у правки тела.
func writeTailscaleBody(node *wizardmodels.Node, ob map[string]interface{}) error {
	bodyBefore := node.Body
	scheme := tailscaleSchemeOfBody(ob)
	if scheme == "" {
		return fmt.Errorf("%s", locale.T("Node has no body to edit"))
	}
	res := nodeflow.Sanitize(scheme, ob)
	if res.Drop != nil {
		if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{*res.Drop})); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("%s", res.Drop.Code)
	}
	if err := awgRejectedInput(res, ob, tailscaleRoleKeys); err != nil {
		return err
	}
	emitted, err := nodeflow.Emit(scheme, res.Clean)
	if err != nil {
		return err
	}
	// Emit снимает managed-ключ "type" — вернуть его обязан пишущий тело.
	stamped, err := config.StampBodyType(scheme, emitted, ob)
	if err != nil {
		return err
	}
	node.Body = stamped
	node.ReplaceDerivedWarnings(nodewarn.FromParsed(res.Warnings))
	// Вердикт ядра привязан к ТЕЛУ: смена роли сменила тело — приговор о
	// прежнем недействителен (SPEC 132, PARSING_PRINCIPLES §9.4).
	node.RevalidateCoreVerdictAfterBodyChange(bodyBefore)
	if node.Origin == nil || node.Origin.Kind == wizardmodels.OriginKindJSON {
		o := wizardmodels.Origin{}
		if node.Origin != nil {
			o = *node.Origin
		}
		o.Kind = wizardmodels.OriginKindJSON
		o.Raw = string(stamped)
		node.Origin = &o
	}
	return nil
}

// tailscaleBlock — блок роли в tailnet в форме узла: виджеты плюс
// перечитывание из узла. Отдельный тип по той же причине, что и awgBlock:
// состояние блока живёт своей жизнью и перечитывается после Regen.
type tailscaleBlock struct {
	advExit  *widget.Check
	exitNode *widget.Entry
	exitLAN  *widget.Check
	// exitRows — строки, которые гасит галка «быть выходом».
	exitRows []fyne.CanvasObject
	// content — всё, что блок добавляет в форму, одним объектом.
	content *fyne.Container
	// syncing — идёт программная запись значений; обработчики не считают её
	// правкой пользователя.
	syncing bool
}

// newTailscaleBlock собирает блок роли для узла.
//
// nodeRef отдаёт узел рабочей копии (правки буферизуются до Save, как и всё
// в этом окне), onApplied зовётся после успешной записи в тело — форма по
// нему перерисовывает вкладку JSON и помечает состояние изменённым.
func newTailscaleBlock(
	nodeRef func() *wizardmodels.Node,
	win fyne.Window,
	onApplied func(),
) *tailscaleBlock {
	b := &tailscaleBlock{}

	b.exitNode = widget.NewEntry()
	b.exitNode.SetPlaceHolder(locale.T("machine name or tailnet IP of the exit node — empty: no exit node"))

	// Правка пишется в узел сразу, без отдельной кнопки: узел живёт в
	// рабочей копии до Save окна, и вторая кнопка «применить» спрашивала бы
	// дважды об одном. Отказ санитайзера показывается диалогом: поля
	// формы — галки и одна строка, посимвольной «недописанности» у них нет.
	commit := func() {
		if b.syncing {
			return
		}
		node := nodeRef()
		if node == nil {
			return
		}
		if err := applyTailscaleRole(node, b.values()); err != nil {
			dialog.ShowError(err, win)
			b.load(node)
			return
		}
		if onApplied != nil {
			onApplied()
		}
	}
	b.exitNode.OnChanged = func(string) { commit() }
	b.exitLAN = widget.NewCheck(locale.T("Keep local network reachable while using the exit node"), func(bool) { commit() })
	b.advExit = widget.NewCheck(locale.T("Advertise this node as an exit node"), func(bool) {
		b.syncExitRole()
		commit()
	})

	note := widget.NewLabel(locale.T(tailscaleBlockNoteText))
	note.Wrapping = fyne.TextWrapWord
	note.Importance = widget.LowImportance

	exitRow := container.NewBorder(nil, nil, widget.NewLabel(locale.T("Exit node")), nil, b.exitNode)
	b.exitRows = []fyne.CanvasObject{exitRow, b.exitLAN}
	// Галка-переключатель роли стоит НАД тем, что от неё зависит: скрытые ею
	// строки лежат ниже, и переключение не двигает саму галку под курсором.
	b.content = container.NewVBox(
		sectionHeader(locale.T("Tailscale")),
		b.advExit,
		exitRow,
		b.exitLAN,
		note,
	)
	return b
}

// values — что сейчас набрано в форме.
func (b *tailscaleBlock) values() tailscaleRole {
	return tailscaleRole{
		AdvertiseExit: b.advExit.Checked,
		ExitNode:      b.exitNode.Text,
		ExitLAN:       b.exitLAN.Checked,
	}
}

// load перечитывает роль из узла: открытие окна, отказ записи, пересборка
// тела кнопкой Regen происхождения или правкой вкладки JSON.
func (b *tailscaleBlock) load(node *wizardmodels.Node) {
	r := readTailscaleRole(node)
	b.syncing = true
	b.advExit.SetChecked(r.AdvertiseExit)
	b.exitNode.SetText(r.ExitNode)
	b.exitLAN.SetChecked(r.ExitLAN)
	b.syncing = false
	b.syncExitRole()
}

// syncExitRole гасит строки чужого выхода, пока узел сам анонсируется
// выходом: ядро обе роли разом не принимает (см. шапку файла).
func (b *tailscaleBlock) syncExitRole() {
	for _, row := range b.exitRows {
		if b.advExit.Checked {
			row.Hide()
			continue
		}
		row.Show()
	}
}
