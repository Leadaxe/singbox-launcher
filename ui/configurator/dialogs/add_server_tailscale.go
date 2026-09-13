// File add_server_tailscale.go — вариант «Tailscale» формы Add server
// (SPEC 122 §2.5).
//
// Отличие от соседних вариантов в том, что узел tailnet — не один outbound.
// Полезен он только вместе с двумя спутниками: DNS-сервером MagicDNS
// (`type: tailscale`, `endpoint` = этот узел) и правилом маршрута на
// 100.64.0.0/10. Порознь их пришлось бы заводить руками на трёх вкладках, и
// пользователь узнавал бы о пропущенном шаге по неработающим именам *.ts.net.
//
// Поэтому форма отдаёт ДОКУМЕНТ узла (SPEC 121 §5.1) — тело плюс секции — в
// AddServerResult.ConfigJSON. Своего пути записи она не заводит: документ
// разбирает тот же AppendManualConfigJSON, что и вкладка JSON окна источника
// (ловушка emitter-parser-pairing — вторая реализация правил документа
// разошлась бы с первой на первой же правке).
package dialogs

import (
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
)

// addServerTailscaleNoteText — подсказка под полем ключа. Текст нормативный
// (SPEC 122 §2.5): одноразовый ключ расходуется первым входом, и дальше
// идентичность машины держит каталог состояния, а не ключ.
const addServerTailscaleNoteText = "One-off keys are consumed at first login; the node identity then lives in the state directory. Deleting the profile registers a new device — use a reusable key for that."

// tailscaleDefaultTag — тег по умолчанию. Тег обязателен: по нему называется
// каталог состояния узла, и на него же ссылаются DNS-сервер и правило
// маршрута из секций.
const tailscaleDefaultTag = "tailscale"

// tailscaleDNSTag — тег DNS-сервера MagicDNS в секциях узла. Локальный: на
// сборке он получает префикс финального тега узла, поэтому два узла tailnet
// своими серверами не сталкиваются.
const tailscaleDNSTag = "ts-dns"

// tailscaleCGNATRange — адресное пространство tailnet (RFC 6598, CGNAT).
const tailscaleCGNATRange = "100.64.0.0/10"

// tailscaleMagicDNSSuffix — доменный суффикс MagicDNS.
const tailscaleMagicDNSSuffix = ".ts.net"

// tailscaleFields — виджеты варианта.
type tailscaleFields struct {
	authKey    *widget.Entry
	controlURL *widget.Entry
	hostname   *widget.Entry
	ephemeral  *widget.Check
	acceptRts  *widget.Check
	exitNode   *widget.Entry
	box        *fyne.Container
}

// buildTailscaleFields собирает блок полей. onChange зовётся на каждую правку
// — им форма пересчитывает превью JSON.
func buildTailscaleFields(onChange func()) *tailscaleFields {
	t := &tailscaleFields{}

	// Ключ — секрет: password entry. Маскируется только показ; в теле узла и
	// в бэкапе он лежит открыто, как ключи WireGuard («секреты в state — by
	// design»), и в логи тело узла не пишется.
	t.authKey = widget.NewPasswordEntry()
	t.authKey.SetPlaceHolder("tskey-auth-…") // l10n-exempt: key prefix
	t.authKey.OnChanged = func(string) { onChange() }

	mk := func(placeholder string) *widget.Entry {
		e := widget.NewEntry()
		e.SetPlaceHolder(placeholder)
		e.OnChanged = func(string) { onChange() }
		return e
	}
	t.controlURL = mk(locale.T("optional — the official coordination server by default"))
	t.hostname = mk(locale.T("optional — this machine's name by default"))
	t.exitNode = mk(locale.T("optional"))

	t.ephemeral = widget.NewCheck(locale.T("Ephemeral node (removed from the tailnet when it goes offline)"), func(bool) { onChange() })
	t.acceptRts = widget.NewCheck(locale.T("Accept routes advertised by other nodes"), func(bool) { onChange() })

	note := widget.NewLabel(locale.T(addServerTailscaleNoteText))
	note.Wrapping = fyne.TextWrapWord

	t.box = container.NewVBox(
		labeledRow(locale.T("Auth key"), t.authKey),
		note,
		labeledRow(locale.T("Control URL"), t.controlURL),
		labeledRow(locale.T("Hostname"), t.hostname),
		t.ephemeral,
		t.acceptRts,
		labeledRow(locale.T("Exit node"), t.exitNode),
	)
	t.box.Hide()
	return t
}

// tailscaleDocument собирает документ узла: endpoint плюс секции dns/route.
//
// Ссылки на сам узел пишутся `@self` — тем же способом, каким их пишет
// разбор документа (SPEC 121 §5.1): тег узла переживёт переименование, а
// связка с ним нет, если бы её записали буквальным тегом.
func tailscaleDocument(tag string, t *tailscaleFields) ([]byte, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = tailscaleDefaultTag
	}
	key := strings.TrimSpace(t.authKey.Text)
	if key == "" {
		return nil, fmt.Errorf("%s", locale.T("Auth key is required"))
	}

	endpoint := map[string]interface{}{
		"type":     "tailscale",
		"tag":      tag,
		"auth_key": key,
	}
	putIfNotEmpty(endpoint, "control_url", t.controlURL.Text)
	putIfNotEmpty(endpoint, "hostname", t.hostname.Text)
	if t.ephemeral.Checked {
		endpoint["ephemeral"] = true
	}
	if t.acceptRts.Checked {
		endpoint["accept_routes"] = true
	}
	putIfNotEmpty(endpoint, "exit_node", t.exitNode.Text)

	doc := map[string]interface{}{
		"endpoints": []interface{}{endpoint},
		"dns": map[string]interface{}{
			"servers": []interface{}{map[string]interface{}{
				"type":     "tailscale",
				"tag":      tailscaleDNSTag,
				"endpoint": "@self",
			}},
			"rules": []interface{}{map[string]interface{}{
				"domain_suffix": []interface{}{tailscaleMagicDNSSuffix},
				"server":        tailscaleDNSTag,
			}},
		},
		"route": map[string]interface{}{
			"rules": []interface{}{map[string]interface{}{
				"ip_cidr":  []interface{}{tailscaleCGNATRange},
				"outbound": "@self",
			}},
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}

// putIfNotEmpty пишет строковое поле, только когда оно заполнено: пустая
// строка в control_url/hostname — не «по умолчанию», а явное пустое значение,
// на котором ядро спотыкается.
func putIfNotEmpty(m map[string]interface{}, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		m[key] = v
	}
}
