package fynewidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
)

// Предупреждение «в ссылке приватный ключ» — один хелпер на все точки выдачи
// ссылки (решение владельца 18.09.2026, «как в LxBox»).
//
// Признак приходит из core/config (ShareURICarriesPrivateKey по телу узла
// либо ShareURITextCarriesPrivateKey по строке ссылки), здесь — только вопрос
// и передача ответа. Пароли и uuid ссылку не задерживают: предупреждать о
// каждой значит приучить жать «Да» не читая.
//
// Живёт здесь, а не в ui: копирование ссылок идёт и из ui, и из
// ui/configurator/tabs, а tabs импортировать ui не может. Тексты и ключи
// locale — те же, что были у прежнего хелпера в ui.
const (
	shareURISecretWarnText     = "This link contains the node's private key. Anyone who gets it will be able to connect as you. Copy?"
	shareURISecretWarnManyText = "Some of these links contain node private keys. Anyone who gets them will be able to connect as you. Copy?"
)

// ConfirmShareURISecret зовёт onOK либо сразу (ключа в ссылке нет), либо после
// подтверждения. Отказ — тишина: ничего не копируем, статус не трогаем.
//
// Вызов обязан идти из UI-потока (fyne.Do) — как и всякий показ диалога.
func ConfirmShareURISecret(win fyne.Window, carriesPrivateKey bool, onOK func()) {
	confirmShareURISecretText(win, carriesPrivateKey, shareURISecretWarnText, onOK)
}

// ConfirmShareURISecretBulk — та же развилка для массового копирования: одно
// подтверждение на всю операцию, текст во множественном числе.
func ConfirmShareURISecretBulk(win fyne.Window, carriesPrivateKey bool, onOK func()) {
	confirmShareURISecretText(win, carriesPrivateKey, shareURISecretWarnManyText, onOK)
}

// ConfirmShareURISecretCopy — самая частая форма: спросить и положить строку в
// буфер. Признак считается по САМОЙ строке (ShareURITextCarriesPrivateKey у
// вызывающего), тела узла тут нет.
func ConfirmShareURISecretCopy(win fyne.Window, carriesPrivateKey bool, text string) {
	ConfirmShareURISecret(win, carriesPrivateKey, func() { SetClipboard(text) })
}

func confirmShareURISecretText(win fyne.Window, carriesPrivateKey bool, text string, onOK func()) {
	if onOK == nil {
		return
	}
	if !carriesPrivateKey || win == nil {
		onOK()
		return
	}
	// Label без Wrapping растягивает окно по длине строки (его single-line
	// width = min-width и переопределяет размер диалога) — текст здесь
	// длинный, поэтому перенос обязателен.
	msg := widget.NewLabel(locale.T(text))
	msg.Wrapping = fyne.TextWrapWord
	body := container.NewVBox(msg)
	d := dialog.NewCustomConfirm(
		locale.T("Private key in the link"),
		locale.T("Copy"),
		locale.T("Cancel"),
		body,
		func(ok bool) {
			if ok {
				onOK()
			}
		},
		win,
	)
	d.Resize(fyne.NewSize(460, 200))
	d.Show()
}
