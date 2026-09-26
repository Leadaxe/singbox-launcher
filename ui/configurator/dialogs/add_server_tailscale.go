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

	"singbox-launcher/core/config/nodeflow"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
)

// addServerTailscaleNoteText — подсказка под полем ключа. Текст нормативный
// (SPEC 122 §2.5): одноразовый ключ расходуется первым входом, и дальше
// идентичность машины держит каталог состояния, а не ключ.
const addServerTailscaleNoteText = "One-off keys are consumed at first login; the node identity then lives in the state directory. Deleting the profile registers a new device — use a reusable key for that."

// addServerTailscaleAdvertiseNoteText — подсказка под блоком анонсов.
// Нормативна ровно в одном: анонс сам по себе ничего не включает. Ядро шлёт
// его координатору, а разрешение узел получает в админке tailnet — без этого
// шага галка выглядит сработавшей, а трафик не идёт.
const addServerTailscaleAdvertiseNoteText = "Advertised routes and exit nodes stay pending until they are approved in the tailnet admin console."

// tailscaleDefaultTag — тег по умолчанию. Тег обязателен: по нему называется
// каталог состояния узла, и на него же ссылаются DNS-сервер и правило
// маршрута из секций.
const tailscaleDefaultTag = "tailscale"

// tailscaleFields — виджеты варианта.
type tailscaleFields struct {
	authKey     *widget.Entry
	controlURL  *widget.Entry
	hostname    *widget.Entry
	ephemeral   *widget.Check
	acceptRts   *widget.Check
	exitNode    *widget.Entry
	exitNodeLAN *widget.Check
	advExit     *widget.Check
	advRoutes   *widget.Entry
	advTags     *widget.Entry
	box         *fyne.Container

	// exitNodeRow держится отдельно от box, чтобы гасить строку целиком
	// вместе с её подписью (§ «две роли исключают друг друга» ниже).
	exitNodeRow *fyne.Container
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

	t.advRoutes = mk(locale.T("optional — CIDR list, comma-separated"))
	t.advTags = mk(locale.T("optional — tag:name list, comma-separated"))

	t.ephemeral = widget.NewCheck(locale.T("Ephemeral node (removed from the tailnet when it goes offline)"), func(bool) { onChange() })
	t.acceptRts = widget.NewCheck(locale.T("Accept routes advertised by other nodes"), func(bool) { onChange() })
	t.exitNodeLAN = widget.NewCheck(locale.T("Keep local network reachable while using the exit node"), func(bool) { onChange() })

	// Две роли исключают друг друга: ядро отказывается стартовать с
	// `advertise_exit_node` и непустым `exit_node` разом (protocol/tailscale
	// /endpoint.go — «cannot advertise an exit node and use an exit node at
	// the same time»). Проверка живёт в NewEndpoint, то есть `sing-box check`
	// её не ловит — падает только запуск (ловушка
	// chain-check-misses-start-errors). Поэтому форма не даёт собрать такую
	// пару вовсе: галка «быть выходом» гасит строку «пользоваться выходом».
	t.advExit = widget.NewCheck(locale.T("Advertise this node as an exit node"), func(bool) {
		t.syncExitRole()
		onChange()
	})

	note := widget.NewLabel(locale.T(addServerTailscaleNoteText))
	note.Wrapping = fyne.TextWrapWord

	advNote := widget.NewLabel(locale.T(addServerTailscaleAdvertiseNoteText))
	advNote.Wrapping = fyne.TextWrapWord

	t.exitNodeRow = labeledRow(locale.T("Exit node"), t.exitNode)

	t.box = container.NewVBox(
		labeledRow(locale.T("Auth key"), t.authKey),
		note,
		labeledRow(locale.T("Control URL"), t.controlURL),
		labeledRow(locale.T("Hostname"), t.hostname),
		t.ephemeral,
		t.acceptRts,
		widget.NewSeparator(),
		// Галка-переключатель роли стоит НАД тем, что от неё зависит: скрытые
		// ею строки лежат ниже, поэтому переключение не двигает саму галку
		// под курсором — уезжает только хвост формы.
		t.advExit,
		labeledRow(locale.T("Advertise routes"), t.advRoutes),
		labeledRow(locale.T("ACL tags"), t.advTags),
		t.exitNodeRow,
		t.exitNodeLAN,
		advNote,
	)
	t.syncExitRole()
	t.box.Hide()
	return t
}

// syncExitRole разводит две роли узла в tailnet, которые ядро вместе не
// принимает: «быть выходом» (advertise_exit_node) и «пользоваться чужим
// выходом» (exit_node). Галка гасит строку чужого выхода и её спутника
// exit_node_allow_lan_access — тот и сам по себе бессмыслен без exit_node:
// ядро применяет его только внутри ветки `if t.exitNode != ""`.
func (t *tailscaleFields) syncExitRole() {
	if t.advExit.Checked {
		t.exitNodeRow.Hide()
		t.exitNodeLAN.Hide()
		return
	}
	t.exitNodeRow.Show()
	t.exitNodeLAN.Show()
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
	// Роли взаимоисключающие (см. syncExitRole): на анонсе чужой выход в тело
	// не пишется, даже если строка осталась заполненной с прошлого состояния
	// галки.
	if t.advExit.Checked {
		endpoint["advertise_exit_node"] = true
	} else {
		putIfNotEmpty(endpoint, "exit_node", t.exitNode.Text)
		// Связь с exit_node судит реестр (requires у
		// exit_node_allow_lan_access) — через tailscaleVerdict ниже.
		if t.exitNodeLAN.Checked {
			endpoint["exit_node_allow_lan_access"] = true
		}
	}

	// Маршруты судит и приводит реестр (контракт 1.1.63, SPEC 142 C9):
	// формат CIDR, голый адрес → префикс хоста, биты хоста за префиксом
	// обнуляются (normalize cidr_masked), дефолтный маршрут запрещён
	// (item_forbidden — для выхода наружу есть галка advertise_exit_node).
	if routes := splitTailscaleList(t.advRoutes.Text); len(routes) > 0 {
		endpoint["advertise_routes"] = routes
	}
	if tags := splitTailscaleList(t.advTags.Text); len(tags) > 0 {
		endpoint["advertise_tags"] = tags
	}
	clean, err := tailscaleVerdict(endpoint)
	if err != nil {
		return nil, err
	}
	// В документ уходит ПРИВЕДЁННОЕ значение: `192.168.10.5/24` ядро
	// отвергло бы, `192.168.10.0/24` — ровно то, что человек имел в виду.
	if routes, ok := clean["advertise_routes"]; ok {
		endpoint["advertise_routes"] = routes
	}

	// Связка — не литерал формы: её собирает config.TailscaleBundleFragments,
	// та же функция, которой голый узел получает связку по умолчанию на
	// разборе и на импорте. Свой литерал здесь разошёлся бы с ними на первой
	// же правке нормы (NODE_SECTIONS.md §6).
	frags := corestate.TailscaleBundleFragments()
	doc := map[string]interface{}{
		"endpoints": []interface{}{endpoint},
		"dns": map[string]interface{}{
			"servers": rawList(frags.DNSServers),
			"rules":   rawList(frags.DNSRules),
		},
		"route": map[string]interface{}{
			"rules": rawList(frags.RouteRules),
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}

// tailscaleVerdict прогоняет тело узла через санитайзер реестра: поле, которое
// он снял с кодом уровня warning/error (связи полей, формат), — отказ формы с
// текстом кода, а не узел без набранного значения (SPEC 142 B7).
//
// Первое значение — чистое тело санитайзера: из него форма берёт
// приведённые реестром значения.
func tailscaleVerdict(endpoint map[string]interface{}) (map[string]interface{}, error) {
	res := nodeflow.Sanitize(tailscaleScheme, endpoint)
	ws := res.Warnings
	if res.Drop != nil {
		ws = append([]nodeflow.Warning{*res.Drop}, ws...)
	}
	if msg := nodewarn.Summary(nodewarn.FromParsed(ws)); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	return res.Clean, nil
}

// tailscaleScheme — схема реестра узла Tailscale.
const tailscaleScheme = "tailscale"

// splitTailscaleList режет список, набранный через запятую или пробелы.
func splitTailscaleList(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// putIfNotEmpty пишет строковое поле, только когда оно заполнено: пустая
// строка в control_url/hostname — не «по умолчанию», а явное пустое значение,
// на котором ядро спотыкается.
func putIfNotEmpty(m map[string]interface{}, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		m[key] = v
	}
}

// rawList — список готовых фрагментов как элементы JSON-документа.
// json.RawMessage маршалится телом, поэтому порядок ключей внутри фрагмента
// остаётся тем, каким его собрала норма.
func rawList(list []json.RawMessage) []interface{} {
	out := make([]interface{}, 0, len(list))
	for _, raw := range list {
		out = append(out, raw)
	}
	return out
}
