// File source_form_edit.go — правка готового узла формой схемы в окне узла
// (SPEC 167 §2.3, для любой схемы с формой реестра — SPEC 168 §2.2).
//
// Те же поля и блоки, что у конструктора «Add server»: оба собирают
// wizarddialogs.NodeForm на общей модели wizardbusiness.NodeFormModel
// (раскладка по reg.FormFields схемы). Имени здесь нет — тег узла правится
// своим полем окна.
//
// Форма пишет только свои пути поверх ТЕКУЩЕГО тела узла: cookie, token,
// сертификаты клиента, detour, второй пир WireGuard и прочие ключи из JSON
// остаются как есть. Скрытое visible_when поле из тела не снимается.
// Обфускацию AmneziaWG (блок `amnezia` схемы wireguard) форма здесь не
// рисует: её правит source_awg_edit.go — один редактор на поле (снятие при
// detour, перечитывание после пересборки).
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

// nodeFormEditSkipBlocks — блоки формы, которые в окне узла правит другой
// редактор: обфускация AmneziaWG — блок source_awg_edit.go.
var nodeFormEditSkipBlocks = []string{"amnezia"}

// nodeFormBodyOf — схема и тело узла картой, если у схемы узла (по типу
// тела через реестр) есть форма; "" — нет.
func nodeFormBodyOf(node *wizardmodels.Node) (string, map[string]interface{}) {
	if node == nil || len(node.Body) == 0 {
		return "", nil
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		return "", nil
	}
	t, _ := ob["type"].(string)
	reg, err := registry.Get()
	if err != nil {
		return "", nil
	}
	scheme, ok := reg.NodeSchemeForSingboxType(t)
	if !ok || reg.Form(scheme) == nil || wizardbusiness.NodeFormSchemeLabel(scheme) == "" {
		return "", nil
	}
	return scheme, ob
}

// nodeFormEditableNode — схема узла, если блок правки показывать узлу
// (проверка по ТЕЛУ: узел мог приехать из конструктора, JSON или бэкапа);
// "" — не показывать.
func nodeFormEditableNode(node *wizardmodels.Node) string {
	scheme, _ := nodeFormBodyOf(node)
	return scheme
}

// writeNodeFormBody — запись тела формой тем же конвейером, что у блоков
// AWG и Tailscale: Sanitize → Emit → StampBodyType, коды узла заново,
// вердикт ядра снят (тело сменилось). Отказ уровня узла — ОТКАТ.
//
// У узла с источником-телом (вид `json`) источник правится вместе с телом,
// иначе «Regen from raw» вернул бы прежние поля.
func writeNodeFormBody(node *wizardmodels.Node, scheme string, ob map[string]interface{}) error {
	bodyBefore := node.Body
	res := nodeflow.Sanitize(scheme, ob)
	if res.Drop != nil {
		if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{*res.Drop})); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("%s", res.Drop.Code)
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

// nodeFormBlock — блок правки узла формой его схемы.
type nodeFormBlock struct {
	scheme string
	form   *wizarddialogs.NodeForm
	// status — причина, по которой набранное не записано (пусто — записано).
	status  *widget.Label
	content *fyne.Container
	syncing bool
}

// newNodeFormBlock собирает блок для схемы scheme. nil — у реестра нет
// формы схемы.
//
// nodeRef отдаёт узел рабочей копии, onApplied зовётся после успешной
// записи в тело (перерисовка JSON, пометка изменений).
func newNodeFormBlock(
	scheme string,
	nodeRef func() *wizardmodels.Node,
	onApplied func(),
) *nodeFormBlock {
	b := &nodeFormBlock{scheme: scheme}
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
		// Основа — ТЕКУЩЕЕ тело узла: его мог переписать другой редактор
		// окна (обфускация AWG, вкладка JSON) после загрузки формы.
		if s, ob := nodeFormBodyOf(node); s == b.scheme && ob != nil {
			b.form.SetBase(ob)
		}
		body, err := b.form.Body()
		if err == nil {
			err = b.form.Verdict(body)
		}
		if err == nil {
			err = writeNodeFormBody(node, b.scheme, body)
		}
		if err != nil {
			b.setStatus(err.Error())
			return
		}
		b.setStatus("")
		// Записанное тело — новая основа формы. Виджеты не перезаписываются —
		// человек ещё печатает.
		if _, ob := nodeFormBodyOf(node); ob != nil {
			b.form.SetBase(ob)
		}
		if onApplied != nil {
			onApplied()
		}
	}
	form, err := wizarddialogs.NewNodeForm(scheme, wizardbusiness.NodeFormOptions{SkipBlocks: nodeFormEditSkipBlocks}, commit)
	if err != nil {
		return nil
	}
	b.form = form
	b.content = container.NewVBox(
		sectionHeader(locale.T(wizardbusiness.NodeFormSchemeLabel(scheme))),
		form.Object(),
		b.status,
	)
	return b
}

func (b *nodeFormBlock) setStatus(text string) {
	b.status.SetText(text)
	if text == "" {
		b.status.Hide()
		return
	}
	b.status.Show()
}

// load перечитывает форму из узла: открытие окна, Regen происхождения,
// правка вкладки JSON.
func (b *nodeFormBlock) load(node *wizardmodels.Node) {
	if b == nil || b.form == nil {
		return
	}
	s, ob := nodeFormBodyOf(node)
	if s != b.scheme {
		ob = nil // тело сменило схему — блок этой схемы окно не покажет
	}
	b.syncing = true
	b.form.Load(ob)
	b.syncing = false
	b.setStatus("")
}
