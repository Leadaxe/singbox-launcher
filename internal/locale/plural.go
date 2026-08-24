package locale

// PluralResolver maps a count to a CLDR plural form name for one language.
// Forms() is the full set a translation must provide; the l10n checker
// reads it too, so the list is never duplicated outside this file.
type PluralResolver interface {
	Forms() []string
	Resolve(n int) string
}

// EnPluralResolver implements CLDR plural rules for English: one/other.
// It is also the fallback for any language without its own resolver.
type EnPluralResolver struct{}

func (EnPluralResolver) Forms() []string { return []string{"one", "other"} }

func (EnPluralResolver) Resolve(n int) string {
	if n == 1 || n == -1 {
		return "one"
	}
	return "other"
}

// RuPluralResolver implements CLDR plural rules for Russian:
// one/few/many/other. The API takes integers, so "other" (fractions) never
// fires at runtime, but the form stays in the required set for CLDR
// completeness — a translation must fill it.
type RuPluralResolver struct{}

func (RuPluralResolver) Forms() []string { return []string{"one", "few", "many", "other"} }

func (RuPluralResolver) Resolve(n int) string {
	if n < 0 {
		n = -n
	}
	switch {
	case n%10 == 1 && n%100 != 11:
		return "one"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return "few"
	default:
		return "many"
	}
}

// resolverFor returns the plural resolver for a language tag.
func resolverFor(lang string) PluralResolver {
	if lang == "ru" {
		return RuPluralResolver{}
	}
	return EnPluralResolver{}
}
