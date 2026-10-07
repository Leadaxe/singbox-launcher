// File copy_button.go — кнопка «скопировать» с тихим фидбеком.
//
// Переехала сюда из ui/command_row.go: кнопку копирования рисуют и пакет ui,
// и вкладки Мастера (отчёт сборки), а пакет ui вкладкам не импортировать.
// Реализация одна: три разошедшихся фидбека уже были.
package fynewidget

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// CopyFeedbackDelay — сколько держится галочка после копирования.
const CopyFeedbackDelay = 1200 * time.Millisecond

// NewCopyButton — кнопка «скопировать»: иконка на секунду становится
// галочкой и возвращается обратно.
//
// text вычисляется в момент нажатия, а не при сборке: содержимое пересчитывается
// от состояния формы, и захват строки заранее копировал бы устаревшее
// значение. ok=false — копирования не было, и галочку не показываем: она
// означала бы «в буфере то, что нужно».
//
// tooltip — уже переведённая подсказка; пустая — без подсказки.
func NewCopyButton(tooltip string, text func() (string, bool)) *ttwidget.Button {
	var btn *ttwidget.Button
	btn = ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		s, ok := text()
		if !ok {
			return
		}
		SetClipboard(s)
		btn.SetIcon(theme.ConfirmIcon())
		go func() {
			time.Sleep(CopyFeedbackDelay)
			fyne.Do(func() { btn.SetIcon(theme.ContentCopyIcon()) })
		}()
	})
	if tooltip != "" {
		btn.SetToolTip(tooltip)
	}
	return btn
}
