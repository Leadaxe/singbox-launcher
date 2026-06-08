// Package icons — embedded SVG resources for tab icons that aren't in
// fyne/theme. We tried emoji-in-label first (⚙️ ⚡ etc.) — works for
// most emoji because they include U+FE0F variation selector which forces
// emoji presentation, but Fyne's default font has no glyph for some
// codepoints (notably ⚡ U+26A1 even with VS-16) → tab rendered with
// blank space. Real SVGs via theme.NewThemedResource render reliably and
// inherit the active text color (currentColor).
package icons

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed bolt.svg
var boltSVG []byte

// Bolt — lightning-bolt icon, used as the Core tab indicator. Replaces
// the ⚡ emoji that Fyne's default font couldn't render.
var Bolt fyne.Resource = &fyne.StaticResource{
	StaticName:    "bolt.svg",
	StaticContent: boltSVG,
}

//go:embed telegram.svg
var telegramSVG []byte

// Telegram — blue paper-plane, for Telegram support links (t.me / tg://).
// Kept as a StaticResource (NOT themed) so it stays Telegram-blue regardless
// of the active theme's foreground color.
var Telegram fyne.Resource = &fyne.StaticResource{
	StaticName:    "telegram.svg",
	StaticContent: telegramSVG,
}

//go:embed link.svg
var linkSVG []byte

// Link — generic chain/link icon for non-Telegram support / web-page links.
// Themed (currentColor) so it inherits the active text color.
var Link fyne.Resource = theme.NewThemedResource(&fyne.StaticResource{
	StaticName:    "link.svg",
	StaticContent: linkSVG,
})
