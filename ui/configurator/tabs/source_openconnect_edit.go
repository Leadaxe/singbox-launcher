// File source_openconnect_edit.go — правка готового узла OpenConnect в окне
// узла (SPEC 167 §2.3).
//
// Те же поля и блоки, что у конструктора «Add server → OpenConnect»: оба
// собирают wizarddialogs.OpenConnectForm на общей модели
// wizardbusiness.OpenConnectFormModel (раскладка по reg.Form). Имени здесь
// нет — тег узла правится своим полем окна.
//
// Форма пишет только свои пути поверх текущего тела: cookie, token,
// клиентский сертификат, csd/hip/tncc, detour и прочие ключи из JSON
// остаются как есть. Скрытое visible_when поле из тела не снимается.
//
// Правка пишется в узел рабочей копии сразу, как у блоков AWG и Tailscale
// (до Save окна узел живёт в scratch). Негодный набор в тело не едет и не
// ругается модалкой на каждый символ: причина видна строкой под блоком.
package tabs

import (
	"encoding/json"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizarddialogs "singbox-launcher/ui/configurator/dialogs"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// openConnectBodyOf — тело узла картой, если это узел OpenConnect (схема по
// типу тела через реестр, как у блока Tailscale); nil — не он.
func openConnectBodyOf(node *wizardmodels.Node) map[string]interface{} {
	if node == nil || len(node.Body) == 0 {
		return nil
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return nil
	}
	t, _ := ob["type"].(string)
	reg, err := registry.Get()
	if err != nil {
		return nil
	}
	if scheme, ok := reg.NodeSchemeForSingboxType(t); !ok || scheme != wizardbusiness.OpenConnectScheme {
		return nil
	}
	return ob
}

// openConnectEditableNode — блок правки показывать узлу (проверка по ТЕЛУ:
// узел мог приехать из конструктора, JSON или бэкапа).
func openConnectEditableNode(node *wizardmodels.Node) bool {
	return openConnectBodyOf(node) != nil
}

// writeOpenConnectBody — запись тела формой тем же конвейером, что у блоков
// AWG и Tailscale: Sanitize → Emit → StampBodyType, коды узла заново,
// вердикт ядра снят (тело сменилось). Отказ уровня узла — ОТКАТ.
//
// У узла с источником-телом (вид `json`) источник правится вместе с телом,
// иначе «Regen from raw» вернул бы прежние поля.
func writeOpenConnectBody(node *wizardmodels.Node, ob map[string]interface{}) error {
	bodyBefore := node.Body
	res := nodeflow.Sanitize(wizardbusiness.OpenConnectScheme, ob)
	if res.Drop != nil {
		if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{*res.Drop})); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("%s", res.Drop.Code)
	}
	emitted, err := nodeflow.Emit(wizardbusiness.OpenConnectScheme, res.Clean)
	if err != nil {
		return err
	}
	// Emit снимает managed-ключ "type" — вернуть его обязан пишущий тело.
	stamped, err := config.StampBodyType(wizardbusiness.OpenConnectScheme, emitted, ob)
	if err != nil {
		return err
	}
	node.Body = stamped
	node.ReplaceDerivedWarnings(nodewarn.FromParsed(res.Warnings))
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

// openConnectBlock — блок правки узла OpenConnect.
type openConnectBlock struct {
	form *wizarddialogs.OpenConnectForm
	// status — причина, по которой набранное не записано (пусто — записано).
	status  *widget.Label
	content *fyne.Container
	syncing bool
}

// newOpenConnectBlock собирает блок. nil — у реестра нет формы схемы.
//
// nodeRef отдаёт узел рабочей копии, onApplied зовётся после успешной
// записи в тело (перерисовка JSON, пометка изменений).
func newOpenConnectBlock(
	nodeRef func() *wizardmodels.Node,
	win fyne.Window,
	onApplied func(),
) *openConnectBlock {
	_ = win
	b := &openConnectBlock{}
	b.status = widget.NewLabel("")
	b.status.Wrapping = fyne.TextWrapWord
	b.status.Importance = widget.WarningImportance
	b.status.Hide()

	commit := func() {
		if b.syncing || b.form == nil {
			return
		}
		node := nodeRef()
		if node == nil {
			return
		}
		body, err := b.form.Body()
		if err == nil {
			err = b.form.Verdict(body)
		}
		if err == nil {
			err = writeOpenConnectBody(node, body)
		}
		if err != nil {
			b.setStatus(err.Error())
			return
		}
		b.setStatus("")
		// Записанное тело — новая основа формы: следующие правки ложатся
		// поверх него (скрытые поля, сквозные ключи). Виджеты не
		// перезаписываются — человек ещё печатает.
		if ob := openConnectBodyOf(node); ob != nil {
			b.form.SetBase(ob)
		}
		if onApplied != nil {
			onApplied()
		}
	}
	form, err := wizarddialogs.NewOpenConnectForm(commit)
	if err != nil {
		return nil
	}
	b.form = form
	b.content = container.NewVBox(
		sectionHeader(locale.T("OpenConnect")),
		form.Object(),
		b.status,
	)
	return b
}

func (b *openConnectBlock) setStatus(text string) {
	b.status.SetText(text)
	if text == "" {
		b.status.Hide()
		return
	}
	b.status.Show()
}

// load перечитывает форму из узла: открытие окна, Regen происхождения,
// правка вкладки JSON.
func (b *openConnectBlock) load(node *wizardmodels.Node) {
	if b == nil || b.form == nil {
		return
	}
	b.syncing = true
	b.form.Load(openConnectBodyOf(node))
	b.syncing = false
	b.setStatus("")
}
