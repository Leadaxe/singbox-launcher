// File connections_helpers.go — shared helpers for legacy → canonical
// connection model conversion on Load (миграции старых схем).
//
// SPEC 117 (W4): обратный синк Save (sync_to_connections.go) удалён; вместе с
// ним ушли его частные хелперы (serverLabelFromLegacy, extractURIFragment,
// sprintfServerN, serverConfigJSONKey). Оставшееся используется только
// Load-миграциями старых форматов.
package state

// buildTagSpecFromLegacy возвращает *legacyTagSpec (или nil если все три поля
// пустые). Используется Load-миграцией v4 (legacy_migration.go): у неё на
// входе прежняя ParserConfig-форма, где маска ещё существует.
func buildTagSpecFromLegacy(prefix, postfix, mask string) *legacyTagSpec {
	if prefix == "" && postfix == "" && mask == "" {
		return nil
	}
	return &legacyTagSpec{
		Prefix:  prefix,
		Postfix: postfix,
		Mask:    mask,
	}
}
