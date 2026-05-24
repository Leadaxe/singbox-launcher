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

import (
	v5 "singbox-launcher/core/state/v5"
)

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
	Meta        MetaSection           `json:"meta"`
	Connections v5.ConnectionsSection `json:"connections"`
	Rules       []Rule                `json:"rules"`
	Vars        []v5.SettingVar       `json:"vars,omitempty"`
	DNSOptions  DNSOptions            `json:"dns_options"`
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
