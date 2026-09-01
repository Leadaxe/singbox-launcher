// File source_identity_block.go — блок «Subscription identification» в форме
// подписки: User-Agent и HWID ИМЕННО ЭТОЙ подписки.
//
// # Зачем per-source, если те же настройки есть глобально
//
// Провайдеры делают с этими двумя полями разное, и оба раза одной глобальной
// пары не хватает:
//
//   - ВЕТВЯТ выдачу по User-Agent — одна ссылка отдаёт разным клиентам разные
//     тела (у Liberty под UA лаунчера приезжает sing-box-ветка с прямыми
//     узлами, под UA Happ — Xray-конфиги, включая рабочие BYPASS через
//     socks5-релей). Подменить UA глобально ради одной подписки значит
//     сломать выдачу остальным, которые под нашим UA отвечают правильно;
//   - ПРИВЯЗЫВАЮТ подписку к устройству по HWID и считают лимит устройств.
//     Один HWID на все подписки не даёт развести устройства между разными
//     провайдерами.
//
// # «Как в системе» — явной галкой, а не пустотой
//
// У каждого поля стоит галка наследования. Пустое значение как признак
// «наследовать» читается неоднозначно: у булевых настроек «не отправлять»
// тогда неотличимо от «не задано», а у текстовых пользователь не видит, что
// именно подставится. Снятая галка открывает поле и пишет своё значение;
// поставленная — гасит поле и стирает переопределение (nil/пусто в модели).
package tabs

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	// sourceIdentityHintText — зачем этот блок вообще нужен.
	sourceIdentityHintText = "Some providers return a different node list depending on the client that asks, and bind the subscription to a device by its ID. Leave “as in system” to use the global settings."
	// sourceIdentityAsSystemText — подпись галки наследования.
	sourceIdentityAsSystemText = "as in system"
	// sourceRelaysHintText — что даёт выделение служебных узлов.
	sourceRelaysHintText = "Some providers route a node through an intermediate relay. Off: the relay lives inside its node and stays invisible. On: it becomes a separate node marked with a gear — you can see it, switch it off, and pick it as a chain hop. It is never offered in Directions."
)

// sourceIdentityBlock — виджеты блока и их привязка к рабочей копии формы.
//
// Как и остальная форма источника, правит scratch (буфер окна), а не модель:
// в модель всё уезжает одной записью на Save.
type sourceIdentityBlock struct {
	content *fyne.Container

	uaCheck *widget.Check
	uaEntry *widget.SelectEntry

	sendCheck   *widget.Check
	sendValue   *widget.Check
	hashCheck   *widget.Check
	hashValue   *widget.Check
	hwidCheck   *widget.Check
	hwidEntry   *widget.Entry
	hwidRegenBt *widget.Button

	relaysCheck *widget.Check
}

// newSourceIdentityBlock собирает блок. `srcRef` отдаёт рабочую копию формы;
// nil из него = окно уже закрыто, правка никуда не пишется.
func newSourceIdentityBlock(srcRef func() *corestate.Source) *sourceIdentityBlock {
	b := &sourceIdentityBlock{}

	// --- User-Agent ---------------------------------------------------------
	// Первым пунктом — UA САМОГО ЛАУНЧЕРА: он значение по умолчанию, и без
	// него из списка нельзя было вернуться к дефолту иначе как галкой «как в
	// системе» (обкатка заход 3). Дальше — пресеты чужих клиентов из ассета.
	uaOptions := append([]string{configtypes.BuildSubscriptionUserAgent()},
		subscription.UserAgentPresets...)
	b.uaEntry = widget.NewSelectEntry(uaOptions)
	b.uaEntry.SetPlaceHolder(locale.T("client User-Agent"))
	b.uaEntry.OnChanged = func(s string) {
		if p := srcRef(); p != nil && !b.uaCheck.Checked {
			p.UserAgent = strings.TrimSpace(s)
		}
	}
	b.uaCheck = widget.NewCheck(locale.T(sourceIdentityAsSystemText), func(on bool) {
		if on {
			b.uaEntry.Disable()
			if p := srcRef(); p != nil {
				p.UserAgent = ""
			}
			return
		}
		b.uaEntry.Enable()
		if p := srcRef(); p != nil {
			p.UserAgent = strings.TrimSpace(b.uaEntry.Text)
		}
	})

	// --- Send device ID -----------------------------------------------------
	b.sendValue = widget.NewCheck(locale.T("Send device ID"), func(on bool) {
		if p := srcRef(); p != nil && !b.sendCheck.Checked {
			v := on
			p.SendHWID = &v
		}
	})
	b.sendCheck = widget.NewCheck(locale.T(sourceIdentityAsSystemText), func(on bool) {
		if on {
			b.sendValue.Disable()
			if p := srcRef(); p != nil {
				p.SendHWID = nil
			}
			return
		}
		b.sendValue.Enable()
		if p := srcRef(); p != nil {
			v := b.sendValue.Checked
			p.SendHWID = &v
		}
	})

	// --- Hash device model --------------------------------------------------
	b.hashValue = widget.NewCheck(locale.T("Hash device model (privacy)"), func(on bool) {
		if p := srcRef(); p != nil && !b.hashCheck.Checked {
			v := on
			p.HashDeviceModel = &v
		}
	})
	b.hashCheck = widget.NewCheck(locale.T(sourceIdentityAsSystemText), func(on bool) {
		if on {
			b.hashValue.Disable()
			if p := srcRef(); p != nil {
				p.HashDeviceModel = nil
			}
			return
		}
		b.hashValue.Enable()
		if p := srcRef(); p != nil {
			v := b.hashValue.Checked
			p.HashDeviceModel = &v
		}
	})

	// --- Device ID ----------------------------------------------------------
	b.hwidEntry = widget.NewEntry()
	b.hwidEntry.SetPlaceHolder(locale.T("device ID for this subscription"))
	b.hwidEntry.OnChanged = func(s string) {
		if p := srcRef(); p != nil && !b.hwidCheck.Checked {
			p.HWID = strings.TrimSpace(s)
		}
	}
	// Своё значение генерируется ТЕМ ЖЕ способом, что глобальное — иначе
	// провайдер увидел бы ID другого формата и мог его отвергнуть.
	b.hwidRegenBt = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		b.hwidEntry.SetText(locale.GenerateUUIDv4())
	})
	b.hwidRegenBt.Importance = widget.LowImportance
	b.hwidCheck = widget.NewCheck(locale.T(sourceIdentityAsSystemText), func(on bool) {
		if on {
			b.hwidEntry.Disable()
			b.hwidRegenBt.Disable()
			if p := srcRef(); p != nil {
				p.HWID = ""
			}
			return
		}
		b.hwidEntry.Enable()
		b.hwidRegenBt.Enable()
		if p := srcRef(); p != nil {
			// Своего ID ещё нет — сразу даём отдельный, иначе снятая галка
			// не меняла бы ничего до ручного ввода.
			if strings.TrimSpace(b.hwidEntry.Text) == "" {
				b.hwidEntry.SetText(locale.GenerateUUIDv4())
			}
			p.HWID = strings.TrimSpace(b.hwidEntry.Text)
		}
	})

	// --- Служебные узлы (релеи BYPASS) --------------------------------------
	//
	// Провайдер отдаёт конфиги, где путь к серверу идёт через промежуточный
	// socks5-релей. Выключено: релей живёт внутри тела своего узла и человеку
	// невидим. Включено: он становится отдельным узлом с шестерёнкой — его
	// видно, можно выключить и выбрать позицией цепочки.
	b.relaysCheck = widget.NewCheck(locale.T("Show provider relays as separate nodes"), func(on bool) {
		if p := srcRef(); p != nil {
			p.ExposeRelays = on
		}
	})
	relaysHint := widget.NewLabel(locale.T(sourceRelaysHintText))
	relaysHint.Wrapping = fyne.TextWrapWord
	relaysHint.Importance = widget.LowImportance

	hint := widget.NewLabel(locale.T(sourceIdentityHintText))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	// Две колонки: слева подпись, справа значение с галкой наследования.
	// Раньше блок шёл в столбец, и подписи «User-Agent» / «Device ID» стояли
	// отдельными строками над полями — семь строк высоты там, где хватает
	// четырёх (обкатка заход 3: «компактизируй, сделай в 2 колонки»).
	//
	// Ширину колонки подписей держит самая длинная из них: Border отдаёт
	// левому элементу его MinSize, поэтому поля начинаются на одной линии
	// сами, без хардкода ширины.
	// Подписи — ОДИН экземпляр самой длинной задаёт ширину колонки: у
	// пустой строки MinSize нулевой, и строки-галки уехали бы левее полей.
	// Поэтому распорка меряется по фактическому тексту, а не по «».
	widest := widget.NewLabel(locale.T("Device ID (HWID)"))
	labelWidth := widest.MinSize().Width
	labelCol := func(text string) fyne.CanvasObject {
		l := widget.NewLabel(text)
		l.Alignment = fyne.TextAlignTrailing
		return container.New(layout.NewGridWrapLayout(
			fyne.NewSize(labelWidth, l.MinSize().Height)), l)
	}
	row := func(label string, value fyne.CanvasObject, check *widget.Check) fyne.CanvasObject {
		return container.NewBorder(nil, nil, labelCol(label), check, value)
	}

	b.content = container.NewVBox(
		row(locale.T("User-Agent"), b.uaEntry, b.uaCheck),
		row(locale.T("Device ID (HWID)"),
			container.NewBorder(nil, nil, nil, b.hwidRegenBt, b.hwidEntry), b.hwidCheck),
		// Галки значения сами себе подпись — левая колонка у них пустая, но
		// отступ тот же, иначе они уехали бы под колонку подписей.
		row("", b.sendValue, b.sendCheck),
		row("", b.hashValue, b.hashCheck),
		hint,
		widget.NewSeparator(),
		b.relaysCheck,
		relaysHint,
	)
	return b
}

// syncFromModel заполняет блок из рабочей копии.
//
// Галка «как в системе» стоит ровно тогда, когда переопределения нет; при
// этом поле показывает ГЛОБАЛЬНОЕ значение — чтобы было видно, что именно
// унаследовано, а не пустоту.
func (b *sourceIdentityBlock) syncFromModel(src *corestate.Source, global sourceIdentityDefaults) {
	if b == nil || src == nil {
		return
	}
	ua := strings.TrimSpace(src.UserAgent)
	b.uaCheck.SetChecked(ua == "")
	if ua == "" {
		b.uaEntry.SetText(global.UserAgent)
		b.uaEntry.Disable()
	} else {
		b.uaEntry.SetText(ua)
		b.uaEntry.Enable()
	}

	b.sendCheck.SetChecked(src.SendHWID == nil)
	if src.SendHWID == nil {
		b.sendValue.SetChecked(global.SendHWID)
		b.sendValue.Disable()
	} else {
		b.sendValue.SetChecked(*src.SendHWID)
		b.sendValue.Enable()
	}

	b.hashCheck.SetChecked(src.HashDeviceModel == nil)
	if src.HashDeviceModel == nil {
		b.hashValue.SetChecked(global.HashModel)
		b.hashValue.Disable()
	} else {
		b.hashValue.SetChecked(*src.HashDeviceModel)
		b.hashValue.Enable()
	}

	b.relaysCheck.SetChecked(src.ExposeRelays)

	hw := strings.TrimSpace(src.HWID)
	b.hwidCheck.SetChecked(hw == "")
	if hw == "" {
		b.hwidEntry.SetText(global.HWID)
		b.hwidEntry.Disable()
		b.hwidRegenBt.Disable()
	} else {
		b.hwidEntry.SetText(hw)
		b.hwidEntry.Enable()
		b.hwidRegenBt.Enable()
	}
}

// sourceIdentityDefaults — глобальные значения, которые показываются под
// галкой «как в системе».
type sourceIdentityDefaults struct {
	UserAgent string
	HWID      string
	SendHWID  bool
	HashModel bool
}

// currentIdentityDefaults — что подставится подписке без своих значений.
//
// Читается на каждое заполнение формы, а не кэшируется: глобальные настройки
// правятся в другом окне, и снимок протух бы молча.
func currentIdentityDefaults(execDir string) sourceIdentityDefaults {
	st := locale.LoadSettings(platform.GetBinDir(execDir))
	ua := strings.TrimSpace(st.SubscriptionUserAgent)
	if ua == "" {
		ua = configtypes.BuildSubscriptionUserAgent()
	}
	return sourceIdentityDefaults{
		UserAgent: ua,
		HWID:      st.HWID,
		SendHWID:  st.ShouldSendHWID(),
		HashModel: st.SubscriptionDeviceModelHashed,
	}
}
