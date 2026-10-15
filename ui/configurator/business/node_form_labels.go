// File node_form_labels.go — то, что у формы узла своё (SPEC 168 §2.2):
// подписи и подсказки полей, заголовки блоков, подписи групп выбора и
// пунктов выпадающих списков, названия схем в списке «Add server».
//
// Раскладку (блоки, порядок, видимость, обязательность, варианты) даёт
// реестр — модель (node_form_model.go) её не повторяет. Здесь только тексты:
// ключи locale на английском, по ПУТИ тела, общие для всех схем, плюс
// переопределения по схеме. Имена схем живут только в этом файле.
//
// Поле с `form` без подписи форма не рисует и не трогает (правило SPEC 167):
// так в форму OpenConnect не попадают token.*, сертификаты клиента и УЦ.
package business

// nodeFormSide — тексты одного поля формы.
type nodeFormSide struct {
	// label — подпись строки (ключ locale); пусто — поле не рисуется.
	label string
	// hint — пояснение под строкой; placeholder — подсказка в пустом поле
	// (без неё — значение по умолчанию из реестра).
	hint        string
	placeholder string
	// emptyLabel — подпись пустого пункта выпадающего списка (без неё —
	// «Default» с дефолтом реестра).
	emptyLabel string
	// values — подписи значений enum (имена продуктов не переводятся).
	values map[string]string
	// host — поле несёт адрес сервера: по нему форма называет новый узел,
	// когда тег не задан.
	host bool
	// classicNote — под полем строка о том, что второй фактор недоступен в
	// классическом режиме ядра.
	classicNote bool
}

// nodeFormPathSide — переопределение поля схемы.
type nodeFormPathSide struct {
	path string
	side nodeFormSide
}

// nodeFormSchemeSide — переопределения схемы. ordered — порядок строк внутри
// блока задают переопределения (макет SPEC 167 у OpenConnect), остальные
// поля идут за ними в порядке реестра.
type nodeFormSchemeSide struct {
	ordered bool
	fields  []nodeFormPathSide
}

// nodeFormCommonSides — подписи по пути тела, общие для всех схем. Путь и
// подпись — на разных строках: маркер l10n-key засчитывает ключом каждый
// литерал своей строки.
var nodeFormCommonSides = map[string]nodeFormSide{
	"server": {
		label: "Server", // l10n-key
		host:  true,
	},
	"server_port": {
		label: "Port", // l10n-key
	},
	"uuid": {
		label: "UUID", // l10n-key
	},
	"flow": {
		label: "Flow", // l10n-key
	},
	"username": {
		label: "Username", // l10n-key
	},
	"password": {
		label: "Password", // l10n-key
	},
	"encryption": {
		label: "Encryption", // l10n-key
	},
	"packet_encoding": {
		label: "Packet encoding", // l10n-key
	},
	"path": {
		label: "Path", // l10n-key
	},
	"version": {
		label: "Version", // l10n-key
	},
	"profile": {
		label: "Profile", // l10n-key
	},
	"private_key": {
		label: "Private key", // l10n-key
	},
	"public_key": {
		label: "Public key", // l10n-key
	},
	"address": {
		label: "Address", // l10n-key
	},
	"mtu": {
		label: "MTU", // l10n-key
	},
	"ip": {
		label: "IPv4 address", // l10n-key
	},
	"ipv6": {
		label: "IPv6 address", // l10n-key
	},
	"uri": {
		label: "URI", // l10n-key
	},
	"vhttp": {
		label: "HTTP version", // l10n-key
	},
	"quic": {
		label: "QUIC", // l10n-key
	},
	"quic_congestion_control": {
		label: "Congestion control", // l10n-key
	},
	"server_ports": {
		label: "Port hopping",                         // l10n-key
		hint:  "Ranges, comma-separated: 20000:30000", // l10n-key
	},
	"hop_interval": {
		label: "Hop interval", // l10n-key
		hint:  "e.g. 30s",     // l10n-key
	},
	"up_mbps": {
		label: "Upload, Mbps", // l10n-key
	},
	"down_mbps": {
		label: "Download, Mbps", // l10n-key
	},
	"obfs.type": {
		label:      "Obfuscation", // l10n-key
		emptyLabel: "Off",         // l10n-key
	},
	"obfs.password": {
		label: "Obfuscation password", // l10n-key
	},
	"udp_over_tcp.enabled": {
		label: "UDP over TCP", // l10n-key
	},
	"tls.enabled": {
		label: "TLS", // l10n-key
	},
	"tls.server_name": {
		label: "SNI", // l10n-key
	},
	"tls.insecure": {
		label: "Allow insecure ⚠", // l10n-key
	},
	"tls.alpn": {
		label:       "ALPN",         // l10n-key
		placeholder: "h2, http/1.1", // l10n-exempt: ALPN sample
	},
	"tls.utls.fingerprint": {
		label:      "uTLS fingerprint", // l10n-key
		emptyLabel: "Off",              // l10n-key
	},
	"tls.reality.public_key": {
		label: "REALITY public key", // l10n-key
	},
	"tls.reality.short_id": {
		label: "REALITY short ID", // l10n-key
	},
	"transport.type": {
		label:      "Transport", // l10n-key
		emptyLabel: "None",      // l10n-key
	},
	"transport.path": {
		label: "Path", // l10n-key
	},
	"transport.host": {
		label: "Host", // l10n-key
	},
	"transport.headers.Host": {
		label: "Host header", // l10n-key
	},
	"transport.service_name": {
		label: "Service name", // l10n-key
	},
	"transport.mode": {
		label: "Mode", // l10n-key
	},
	"peers.0.address": {
		label: "Server", // l10n-key
		host:  true,
	},
	"peers.0.port": {
		label: "Port", // l10n-key
	},
	"peers.0.public_key": {
		label: "Peer public key", // l10n-key
	},
	"peers.0.pre_shared_key": {
		label: "Pre-shared key", // l10n-key
	},
	"peers.0.allowed_ips": {
		label: "Allowed IPs", // l10n-key
	},
	"peers.0.persistent_keepalive_interval": {
		label: "Keepalive", // l10n-key
		hint:  "Seconds",   // l10n-key
	},

	// AmneziaWG: имена ключей .conf, не переводятся.
	"jc":   {label: "Jc"},   // l10n-exempt: AmneziaWG key name
	"jmin": {label: "Jmin"}, // l10n-exempt: AmneziaWG key name
	"jmax": {label: "Jmax"}, // l10n-exempt: AmneziaWG key name
	"s1":   {label: "S1"},   // l10n-exempt: AmneziaWG key name
	"s2":   {label: "S2"},   // l10n-exempt: AmneziaWG key name
	"s3":   {label: "S3"},   // l10n-exempt: AmneziaWG key name
	"s4":   {label: "S4"},   // l10n-exempt: AmneziaWG key name
	"h1":   {label: "H1"},   // l10n-exempt: AmneziaWG key name
	"h2":   {label: "H2"},   // l10n-exempt: AmneziaWG key name
	"h3":   {label: "H3"},   // l10n-exempt: AmneziaWG key name
	"h4":   {label: "H4"},   // l10n-exempt: AmneziaWG key name
	"i1":   {label: "I1"},   // l10n-exempt: AmneziaWG key name
	"i2":   {label: "I2"},   // l10n-exempt: AmneziaWG key name
	"i3":   {label: "I3"},   // l10n-exempt: AmneziaWG key name
	"i4":   {label: "I4"},   // l10n-exempt: AmneziaWG key name
	"i5":   {label: "I5"},   // l10n-exempt: AmneziaWG key name
}

// nodeFormSchemeSides — переопределения по схеме.
var nodeFormSchemeSides = map[string]nodeFormSchemeSide{
	// OpenConnect: подписи, подсказки и порядок строк SPEC 167 §2.3.
	"openconnect": {ordered: true, fields: []nodeFormPathSide{
		{path: "flavor", side: nodeFormSide{
			label: "Gateway", // l10n-key
			values: map[string]string{
				"fortinet":   "FortiGate (fortinet)",          // l10n-exempt: product name
				"anyconnect": "Cisco AnyConnect (anyconnect)", // l10n-exempt: product name
				"gp":         "GlobalProtect (gp)",            // l10n-exempt: product name
				"f5":         "F5 (f5)",                       // l10n-exempt: product name
				"pulse":      "Pulse/Ivanti (pulse)",          // l10n-exempt: product name
				"nc":         "Juniper NC (nc)",               // l10n-exempt: product name
			},
		}},
		{path: "server", side: nodeFormSide{
			label: "Server",                                     // l10n-key
			hint:  "host:port or full URL; port 443 if omitted", // l10n-key
			host:  true,
		}},
		{path: "username", side: nodeFormSide{
			label: "Username", // l10n-key
		}},
		{path: "password", side: nodeFormSide{
			label:       "Password",                                                                           // l10n-key
			hint:        "If the gateway asks for a code, you will be prompted on connect (daemon mode only)", // l10n-key
			classicNote: true,
		}},
		{path: "tls.peer_fingerprint", side: nodeFormSide{
			label:       "Pinned fingerprint",                    // l10n-key
			hint:        "Several fingerprints: comma-separated", // l10n-key
			placeholder: "sha256:…",                              // l10n-exempt: fingerprint format sample
		}},
		{path: "tls.server_name", side: nodeFormSide{
			label: "SNI",                                     // l10n-key
			hint:  "Only if it differs from the server host", // l10n-key
		}},
		{path: "auth_group", side: nodeFormSide{
			label: "Group / Realm", // l10n-key
		}},
		{path: "external_auth_disabled", side: nodeFormSide{
			label: "Disable SSO (external auth)", // l10n-key
		}},
		{path: "no_udp", side: nodeFormSide{
			label: "TCP only (no DTLS)",                                    // l10n-key
			hint:  "Use when UDP is blocked and the tunnel keeps dropping", // l10n-key
		}},
		{path: "user_agent", side: nodeFormSide{
			label: "User-Agent",                                        // l10n-key
			hint:  "Empty = core default (FortiGate: Mozilla/5.0 SV1)", // l10n-key
		}},
		{path: "fortinet_host_check.hostcheck", side: nodeFormSide{
			label: "Hostcheck",      // l10n-key
			hint:  "FortiGate only", // l10n-key
		}},
		{path: "mtu", side: nodeFormSide{
			label: "MTU",                                                       // l10n-key
			hint:  "Empty = from gateway; lower it if HTTPS stalls (576…1500)", // l10n-key
		}},
		{path: "dpd_interval", side: nodeFormSide{
			label: "DPD interval",                                            // l10n-key
			hint:  "Keep-alive probe period, e.g. 30s. Empty = from gateway", // l10n-key
		}},
		{path: "reconnect_timeout", side: nodeFormSide{
			label: "Reconnect timeout",                                                      // l10n-key
			hint:  "How long to retry a dropped tunnel, e.g. 5m. Empty = core default (5m)", // l10n-key
		}},
		{path: "ipv6_disabled", side: nodeFormSide{
			label: "Disable IPv6", // l10n-key
		}},
		{path: "allow_insecure_crypto", side: nodeFormSide{
			label: "Allow legacy crypto ⚠",           // l10n-key
			hint:  "Only for old gateways (TLS 1.0)", // l10n-key
		}},
	}},
	// WireGuard: сервер и ключи — в том порядке, в каком их выдаёт панель.
	"wireguard": {ordered: true, fields: []nodeFormPathSide{
		{path: "peers.0.address", side: nodeFormSide{
			label: "Server", // l10n-key
			host:  true,
		}},
		{path: "peers.0.port", side: nodeFormSide{
			label: "Port", // l10n-key
		}},
		{path: "private_key", side: nodeFormSide{
			label: "Private key", // l10n-key
		}},
		{path: "peers.0.public_key", side: nodeFormSide{
			label: "Peer public key", // l10n-key
		}},
		{path: "address", side: nodeFormSide{
			label:       "Address",     // l10n-key
			placeholder: "10.0.0.2/32", // l10n-exempt: CIDR sample
		}},
	}},
}

// nodeFormBlockSide — заголовок и раскрытие блока. Первый блок формы рисуется
// без заголовка и всегда виден; остальные — аккордеоном, раскрыты, кроме
// collapsed.
type nodeFormBlockSide struct {
	title     string
	collapsed bool
}

var nodeFormBlockSides = map[string]nodeFormBlockSide{
	"connection": {},
	"security": {
		title: "Security", // l10n-key
	},
	"tls": {
		title: "TLS", // l10n-key
	},
	"transport": {
		title: "Transport", // l10n-key
	},
	"obfs": {
		title: "Obfuscation", // l10n-key
	},
	"amnezia": {
		title:     "AmneziaWG", // l10n-key
		collapsed: true,
	},
	"advanced": {
		title:     "Advanced", // l10n-key
		collapsed: true,
	},
}

// nodeFormChoiceSide — подпись группы выбора и её вариантов.
type nodeFormChoiceSide struct {
	label   string
	options []nodeFormOptionSide
}

// nodeFormOptionSide — подпись варианта группы выбора.
type nodeFormOptionSide struct {
	id    string
	label string
}

var nodeFormChoiceSides = map[string]nodeFormChoiceSide{
	"server_cert_check": {
		label: "Server certificate", // l10n-key
		options: []nodeFormOptionSide{
			{id: "system",
				label: "System CA"}, // l10n-key
			{id: "pinned",
				label: "Pinned fingerprint"}, // l10n-key
			{id: "insecure",
				label: "Do not verify ⚠"}, // l10n-key
		},
	},
	"security": {
		label: "Security", // l10n-key
		options: []nodeFormOptionSide{
			{id: "none",
				label: "None"}, // l10n-key
			{id: "tls",
				label: "TLS"}, // l10n-key
			{id: "reality",
				label: "REALITY"}, // l10n-key
		},
	},
}

// optionLabel — подпись варианта группы (ключ locale); пусто — нет.
func (c nodeFormChoiceSide) optionLabel(id string) string {
	for _, o := range c.options {
		if o.id == id {
			return o.label
		}
	}
	return ""
}

// NodeFormScheme — тип узла в списке «Add server».
type NodeFormScheme struct {
	Scheme string
	Label  string // ключ locale
}

// NodeFormSchemes — схемы с формой в порядке списка «Add server» (SPEC 168
// §2.1). Tailscale и Source — свои варианты диалога, не модель.
var NodeFormSchemes = []NodeFormScheme{
	{Scheme: "vless",
		Label: "VLESS"}, // l10n-key
	{Scheme: "hysteria2",
		Label: "Hysteria2"}, // l10n-key
	{Scheme: "trojan",
		Label: "Trojan"}, // l10n-key
	{Scheme: "wireguard",
		Label: "WireGuard (AWG)"}, // l10n-key
	{Scheme: "naive",
		Label: "naive"}, // l10n-key
	{Scheme: "masque",
		Label: "masque"}, // l10n-key
	{Scheme: "socks",
		Label: "SOCKS5"}, // l10n-key
	{Scheme: "http",
		Label: "HTTP"}, // l10n-key
	{Scheme: "openconnect",
		Label: "OpenConnect (SSL VPN)"}, // l10n-key
}

// NodeFormSchemeLabel — подпись схемы (ключ locale); пусто — схемы нет в
// списке форм.
func NodeFormSchemeLabel(scheme string) string {
	for _, s := range NodeFormSchemes {
		if s.Scheme == scheme {
			return s.Label
		}
	}
	return ""
}

// nodeFormSideOf — тексты поля схемы: переопределение схемы, иначе общая
// таблица. rank — позиция в упорядоченном переопределении (-1 — нет).
func nodeFormSideOf(scheme, path string) (nodeFormSide, int) {
	if ss, ok := nodeFormSchemeSides[scheme]; ok {
		for i, ps := range ss.fields {
			if ps.path == path {
				if ss.ordered {
					return ps.side, i
				}
				return ps.side, -1
			}
		}
	}
	return nodeFormCommonSides[path], -1
}
