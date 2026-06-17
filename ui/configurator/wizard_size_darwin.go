package configurator

import "fyne.io/fyne/v2"

// macOSMaxWizardHeight — максимальная высота окна визарда на macOS.
//
// На ноутбуках с logical 1280×800 (типичный минимум для macOS 11 Big Sur)
// окно высотой 660px не помещалось: нижний край с навигационными кнопками
// уходил под Dock. 600px гарантированно влезает с запасом на меню-бар,
// заголовок окна и Dock.
const macOSMaxWizardHeight = 600

// clampWizardSize ограничивает высоту окна визарда потолком, который
// помещается на экран Big Sur. Ширина не трогается. Аргумент app оставлен
// для единообразия сигнатуры с non-darwin-заглушкой.
func clampWizardSize(_ fyne.App, width, height float32) fyne.Size {
	if height > macOSMaxWizardHeight {
		height = macOSMaxWizardHeight
	}
	return fyne.NewSize(width, height)
}
