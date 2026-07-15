// File disk_v6.go — on-disk-схема state.json (SPEC 053 + SPEC 056-R-N).
//
// Расширяет v5. Из v5 переиспользуются MetaSection (заменена этим v6-вариантом),
// ConnectionsSection, Source и SubscriptionMeta — они без изменений.
//
// Изменения:
//
//   - state.custom_rules[] → state.rules[] с kind discriminator
//     (preset / inline / srs) — SPEC 053
//   - state.config_params[] удалено — vars живут в preset.body.vars — SPEC 053
//   - state.dns → state.dns_options с flat kind discriminator (template / preset / user)
//     для servers/rules — SPEC 056-R-N. Старое разделение template_servers /
//     extra_servers / extra_rules удалено.
//   - state.vars[] остаётся (глобальные template vars: cert_store, tun, dns_*, ...)
//
// Top-level layout:
//
//	{
//	  "meta":        { version: 6, schema: "presets_v1", ... },
//	  "connections": { ... },         // как в v5
//	  "rules":       [...],           // header/body kind discriminator
//	  "vars":        [...],           // глобальные wizard vars (включая dns_*)
//	  "dns_options": {                // SPEC 056-R-N
//	    "servers": [{kind, tag|ref, enabled, ...body}],
//	    "rules":   [{kind, ref|..., enabled, ...body}]
//	  }
//	}
//
// См. SPECS/053-F-N-PRESET_BUNDLES/SPEC.md, SPECS/056-R-N-DNS_SCHEMA_REDESIGN/SPEC.md.
package state

// SchemaVersionV6 — формат файла state.json, который пишет v6 path.
// Видимо как SchemaVersionV6 пока v5/v6 namespaces co-exist в Phase 2.
// В Phase 5 переименовывается в SchemaVersion (после удаления v5 пакета).
const SchemaVersionV6 = 6

// SchemaName — внутренний идентификатор схемы (хранится в meta.schema).
// Используется для диагностики и future-proof'инга.
const SchemaName = "presets_v1"

// diskStateV6 — корневая модель на диске v6 (SPEC 053 + SPEC 056-R-N).
//
// Используется ТОЛЬКО внутри marshalDiskV6 / parseV6 (приватный — никаких
// внешних callsite'ов).
//
// Изменения vs v5:
//   - meta.version: 5 → 6
//   - meta.schema: новое поле "presets_v1"
//   - custom_rules[] → rules[] с kind discriminator (preset/inline/srs) (SPEC 053)
//   - config_params[] удалено (vars per-preset в body.vars) (SPEC 053)
//   - dns → dns_options (flat kind discriminator) (SPEC 056-R-N)
type diskStateV6 struct {
	Meta         MetaSection          `json:"meta"`
	Connections  ConnectionsSection   `json:"connections"`
	Rules        []Rule               `json:"rules"`
	Vars         []SettingVar         `json:"vars,omitempty"`
	DNSOptions   DNSOptions           `json:"dns_options"`
	WarpAccounts *WarpAccountsSection `json:"warp_accounts,omitempty"`
}

// WarpAccountsSection — кеш выданных Cloudflare регистраций WARP.
//
// Зачем: Cloudflare привязывает выданные адреса к ключу, а каждая регистрация —
// это новый ключ со своим IPv6. Без кеша «Add WARP», нажатый дважды (например
// MASQUE H2 и H3), создаёт две независимые регистрации; в LxBox на телефоне обе
// ноды сидят на одной. Кеш даёт то же поведение и не плодит device-записи в
// Cloudflare на каждое открытие визарда.
//
// WG и MASQUE раздельно: у них разные типы ключей (X25519 против ECDSA P-256),
// одной записью не покрыть.
//
// omitempty: у кого WARP не заводили — секции в файле просто нет.
type WarpAccountsSection struct {
	// WG — кешированная WireGuard-регистрация (nil = не заводили).
	WG *WarpWGAccount `json:"wg,omitempty"`
	// Masque — кешированная MASQUE-регистрация (nil = не заводили).
	Masque *WarpMasqueAccount `json:"masque,omitempty"`
}

// WarpWGAccount — снимок warp.Account, достаточный чтобы собрать ноду заново.
// Приватный ключ и токен — секреты; они и так лежат в state.json внутри URI
// источника, эта секция не добавляет новый класс данных на диск.
type WarpWGAccount struct {
	PrivateKey string `json:"private_key"`
	PeerPublic string `json:"peer_public"`
	ClientV4   string `json:"client_v4"`
	ClientV6   string `json:"client_v6"`
	ClientID   string `json:"client_id,omitempty"`
	DeviceID   string `json:"device_id,omitempty"`
	Token      string `json:"token,omitempty"`
	AccountID  string `json:"account_id,omitempty"`
	License    string `json:"license,omitempty"`
	WarpPlus   bool   `json:"warp_plus,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// WarpMasqueAccount — снимок warp.MasqueAccount.
//
// Транспорт (network), sni и таймауты сюда НЕ входят: это параметры ноды, а не
// регистрации — H2 и H3 строятся из одной записи с разным network.
type WarpMasqueAccount struct {
	PrivateKeyDER string `json:"private_key_der"`
	ServerPubDER  string `json:"server_pub_der"`
	ClientV4      string `json:"client_v4"`
	ClientV6      string `json:"client_v6"`
	Server        string `json:"server"`
	Port          int    `json:"port,omitempty"`
	DeviceID      string `json:"device_id,omitempty"`
	Token         string `json:"token,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// MetaSection — мета v6 (заменяет v5.MetaSection). Добавлено поле Schema для
// будущего versioning'а.
type MetaSection struct {
	Version   int    `json:"version"`
	Schema    string `json:"schema,omitempty"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
