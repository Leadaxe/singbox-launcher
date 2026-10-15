package tabs

import (
	"encoding/json"
	"strings"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// tunnelIngressTypes — типы клиентских endpoint'ов, чей входящий поток ядро
// отдаёт в роутер (SPEC 170 §1): ровно `#filter` пресета `tunnel-ingress`.
// Tailscale исключён (контракт 1.1.129, D-136): его пиров ограничивают ACL
// tailnet и фильтр control-plane.
var tunnelIngressTypes = map[string]bool{
	"wireguard":      true,
	"openvpn-client": true,
	"openconnect":    true,
}

// Подписи и подсказки переключателей полей записи allow_* (D-134).
const (
	tunnelAccessTitle = "Incoming connections from this tunnel"
	tunnelAccessHint  = "Peers on the other side of this tunnel can open connections to this device. Without the switches below they are rejected."
	allowLocalText    = "Allow access to this device"
	allowLocalTip     = "Peers reach services on this device's tunnel address. The service sees them as 127.0.0.1, so everything that trusts localhost becomes reachable."
	allowLanText      = "Expose local network"
	allowLanTip       = "Peers reach private networks behind this device."
	allowGatewayText  = "Allow as gateway"
	allowGatewayTip   = "Peers route any traffic through this device; it leaves through the outbound chosen in the Tunnel ingress preset."
)

// tunnelAccessBlock — переключатели allow_local / allow_lan / allow_gateway
// формы узла-сервера (SPEC 170, контракт 1.1.126; без Tailscale — 1.1.129).
// Виден у своего сервера и члена папки (у узла подписки записи нет) с типом
// тела клиентского туннеля. Правит рабочую копию узла; в модель она уходит по Save окна
// целиком. nil — показывать нечего.
func tunnelAccessBlock(m *wizardmodels.WizardModel, link wizardmodels.NodeLink, node *wizardmodels.Node) fyne.CanvasObject {
	if m == nil || node == nil || node.Kind != wizardmodels.SourceKindServer || inSubscription(m, link) {
		return nil
	}
	typ := nodeBodyType(node)
	if !tunnelIngressTypes[typ] {
		return nil
	}
	debuglog.DebugLog("tunnelAccessBlock: node %q type %s", link.Tag, typ)
	// Подписи переводятся на месте вызова: сканер l10n резолвит константы
	// файла только в аргументе locale.T.
	check := func(text, tip string, field *bool) fyne.CanvasObject {
		c := ttwidget.NewCheck(text, func(on bool) { *field = on })
		c.SetChecked(*field)
		c.SetToolTip(tip)
		return c
	}
	title := widget.NewLabel(locale.T(tunnelAccessTitle))
	title.TextStyle = fyne.TextStyle{Bold: true}
	hint := widget.NewLabel(locale.T(tunnelAccessHint))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance
	return container.NewVBox(title, hint,
		check(locale.T(allowLocalText), locale.T(allowLocalTip), &node.AllowLocal),
		check(locale.T(allowLanText), locale.T(allowLanTip), &node.AllowLan),
		check(locale.T(allowGatewayText), locale.T(allowGatewayTip), &node.AllowGateway),
	)
}

// inSubscription — узел адресован в подписку (записи у него нет).
func inSubscription(m *wizardmodels.WizardModel, link wizardmodels.NodeLink) bool {
	fid := strings.TrimSpace(link.FolderID)
	if fid == "" {
		return false
	}
	for i := range m.Sources {
		if m.Sources[i].ID == fid && m.Sources[i].Kind == wizardmodels.SourceKindSubscription {
			return true
		}
	}
	return false
}

// nodeBodyType — `type` тела узла; тело не разбирается — "".
func nodeBodyType(node *wizardmodels.Node) string {
	var body struct {
		Type string `json:"type"`
	}
	if len(node.Body) == 0 || json.Unmarshal(node.Body, &body) != nil {
		return ""
	}
	return body.Type
}
