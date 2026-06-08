// File connections_helpers.go — shared helpers for legacy <-> canonical
// connection model conversion.
package state

// buildTagSpecFromLegacy возвращает *TagSpec (или nil если все три поля пустые).
func buildTagSpecFromLegacy(prefix, postfix, mask string) *TagSpec {
	if prefix == "" && postfix == "" && mask == "" {
		return nil
	}
	return &TagSpec{
		Prefix:  prefix,
		Postfix: postfix,
		Mask:    mask,
	}
}
