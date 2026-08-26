// File servers_node_info_chain.go — секция цепочки в окне «Info» (SPEC 110).
//
// Почему здесь, а не в редакторе цепочки: редактор правит КОНФИГ, а его
// правки доезжают до ядра только через сохранение и пересборку. Замерять
// там значило бы показывать цифры прежнего маршрута рядом с изменённым
// списком позиций — правдоподобные и не про то, что на экране. Здесь же
// строка соответствует работающему ядру по построению.
package ui

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// chainProbeWarmUpRuns — сколько прогонов делаем на одно нажатие.
//
// Два, а не один: первый поднимает туннели (WG-хендшейк, QUIC-сессия) и
// завышен в разы — у наблюдавшихся цепочек 3824 против 1224 мс на той же
// позиции. Показываем второй; первый нигде не отображается, иначе цифры
// «скакали» бы без объяснения.
const chainProbeWarmUpRuns = 2

// chainLayerResult — результат замера одной позиции.
type chainLayerResult struct {
	DelayMs int64
	// Error — текст ЯДРА. Оно формулирует его само, с указанием позиции и
	// пути до неё, поэтому показывается целиком и не переписывается.
	Error string
	// Skipped — позиция схлопнута (direct), мерить нечего.
	Skipped bool
}

// addChainSection дорисовывает секцию цепочки, если узел ею является.
//
// Как секция пула: сперва спрашиваем ядро и лишь при непустом ответе создаём
// содержимое. Цепочка, которой в рантайме нет (переименовали, не пересобрали
// конфиг), не должна оставлять в окне пустой заголовок.
func addChainSection(ac *core.AppController, body *fyne.Container, win fyne.Window, tag string) {
	if ac == nil || body == nil || !ac.ChainsAvailable() {
		return
	}
	chainBox := container.NewVBox()
	body.Add(chainBox)

	go func(chainTag string) {
		info, ok := ac.ChainFor(chainTag)
		if !ok || len(info.Positions) == 0 {
			return
		}
		fyne.Do(func() {
			buildChainSection(ac, chainBox, win, info)
			chainBox.Refresh()
		})
	}(tag)
}

// buildChainSection рисует позиции и кнопку замера.
func buildChainSection(ac *core.AppController, box *fyne.Container, win fyne.Window, info core.ChainInfo) {
	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.Tf("Chain positions (%d)", len(info.Positions))))

	// Строка на позицию: слева тумблер, затем состав, справа замер. Галочка
	// слева читается как «хоп участвует в маршруте» — список позиций и есть
	// маршрут; правый край остаётся за задержками, иначе тумблер и цифра
	// дрались бы за одно место.
	rows := make([]*widget.Label, len(info.Positions))
	delays := make([]*widget.Label, len(info.Positions))
	toggles := make([]*widget.Check, len(info.Positions))
	// applying — идёт программная расстановка галочек, а не клик
	// пользователя. Fyne зовёт OnChanged и на SetChecked, и отличить одно
	// от другого больше нечем.
	applying := false

	// Текст ошибки — под списком, с переносом: сообщение ядра длинное и
	// в колонку задержки не помещается. Объявлен до строк, потому что
	// обработчик тумблера пишет в него провал прогрева.
	errLabel := widget.NewLabel("")
	errLabel.Wrapping = fyne.TextWrapWord
	errLabel.Importance = widget.DangerImportance
	errLabel.Hide()

	for i, pos := range info.Positions {
		rows[i] = widget.NewLabel(chainPositionText(i, pos))
		rows[i].Truncation = fyne.TextTruncateEllipsis

		delays[i] = widget.NewLabel("")
		delays[i].Alignment = fyne.TextAlignTrailing

		toggles[i] = newChainPositionToggle(ac, info.Tag, i, &applying, func(fresh core.ChainInfo) {
			// Перерисовываем ВСЮ секцию, а не одну строку: маршрут общий,
			// и выключенный хоп меняет то, во что резолвятся соседние
			// позиции (группа выше могла держать выбор через него).
			applyChainRows(fresh, rows, toggles, &applying)
		}, delays, errLabel)

		box.Add(container.NewBorder(nil, nil, toggles[i], delays[i], rows[i]))
	}
	// Начальное состояние ставим ПОСЛЕ создания тумблеров и под флагом:
	// SetChecked дёргает OnChanged, и без него отрисовка окна отправила бы
	// ядру тумблер, которого пользователь не нажимал.
	applying = true
	for i, pos := range info.Positions {
		toggles[i].SetChecked(!pos.Disabled)
	}
	applying = false
	box.Add(errLabel)

	var probeBtn *widget.Button
	probeBtn = widget.NewButtonWithIcon(
		locale.T("Probe by position"), theme.ViewRefreshIcon(), func() {
			probeBtn.Disable()
			probeBtn.SetText(locale.T("Measuring…"))
			for _, d := range delays {
				d.SetText("")
			}
			errLabel.Hide()

			go func() {
				// Состав перечитываем ПЕРЕД замером, а не берём тот, что
				// прочитали при открытии окна: позиция-группа выбирает
				// участника на лету, и пользователь мог переключить его в
				// соседнем списке. Иначе «Замерить снова» показывало бы
				// задержку до узла, через который трафик уже не идёт, —
				// а именно смена пути без перезапуска и есть то, ради чего
				// цепочку ведут через группу.
				cur := info
				if fresh, ok := ac.ChainFor(info.Tag); ok && len(fresh.Positions) > 0 {
					cur = fresh
				}
				results := probeChainLayers(ac, cur)
				fyne.Do(func() {
					// Строки состава тоже обновляем: если выбор группы
					// сменился, показать старый тег рядом со свежей
					// задержкой значило бы соврать вдвойне.
					for i := range rows {
						if i < len(cur.Positions) {
							rows[i].SetText(chainPositionText(i, cur.Positions[i]))
						}
					}
					applyChainProbeResults(results, delays, errLabel)
					probeBtn.Enable()
					probeBtn.SetText(locale.T("Probe again"))
				})
			}()
		})
	box.Add(container.NewCenter(probeBtn))
}

// newChainPositionToggle — тумблер одной позиции (SPEC 075 ядра).
//
// Состояние живёт в ЯДРЕ (cache-file, по тегам позиций), не в нашем state:
// лаунчер здесь только пульт. Поэтому после ответа ядра состав перечитываем
// через refresh, а не рисуем то, что нажали, — если ядро переключение не
// приняло, галочка обязана вернуться, а не соврать.
func newChainPositionToggle(
	ac *core.AppController,
	chainTag string,
	pos int,
	applying *bool,
	refresh func(core.ChainInfo),
	delays []*widget.Label,
	errLabel *widget.Label,
) *widget.Check {
	var check *widget.Check
	check = widget.NewCheck("", func(enabled bool) {
		if *applying {
			return
		}
		check.Disable()
		errLabel.Hide()
		// Все замеры протухли разом: путь через позицию i входит в цену
		// каждой позиции выше. Цифра прежнего маршрута рядом с новым
		// составом врала бы, и заметить это было бы нечем.
		for _, d := range delays {
			d.SetText("")
		}

		go func() {
			warmupErr, err := ac.SetChainPositionEnabled(chainTag, pos, enabled)
			// Состояние перечитываем здесь же, в фоне: ChainFor — это RPC к
			// ядру, и его дедлайн в UI-потоке подвесил бы окно целиком.
			fresh, ok := ac.ChainFor(chainTag)
			fyne.Do(func() {
				defer check.Enable()
				switch {
				case err != nil:
					debuglog.WarnLog("chain toggle: %s#%d enabled=%v: %v", chainTag, pos, enabled, err)
					errLabel.SetText(chainToggleErrorText(err))
					errLabel.Show()
				case strings.TrimSpace(warmupErr) != "":
					// Флаг ядро применило, а звено не поднялось. Это диагноз
					// узла, а не отказ переключения: галочка остаётся там,
					// куда её поставил пользователь, текст объясняет, почему
					// трафик через позицию пока не пойдёт.
					debuglog.WarnLog("chain toggle warmup: %s#%d: %s", chainTag, pos, warmupErr)
					errLabel.SetText(warmupErr)
					errLabel.Show()
				}
				if ok {
					refresh(fresh)
				}
			})
		}()
	})
	return check
}

// applyChainRows приводит строки с галочками к состоянию, прочитанному у
// ядра. Состояние уже на руках — сюда попадаем из UI-потока, ходить в ядро
// отсюда нельзя.
//
// Под флагом applying: SetChecked зовёт OnChanged, и без него приведение к
// состоянию ядра само отправило бы ядру новый тумблер — рекурсией.
func applyChainRows(
	fresh core.ChainInfo,
	rows []*widget.Label,
	toggles []*widget.Check,
	applying *bool,
) {
	*applying = true
	defer func() { *applying = false }()
	for i := range rows {
		if i >= len(fresh.Positions) {
			break
		}
		rows[i].SetText(chainPositionText(i, fresh.Positions[i]))
		toggles[i].SetChecked(!fresh.Positions[i].Disabled)
	}
}

// chainToggleErrorText — текст отказа переключения.
//
// Старое ядро выделено: оно отвечает Unimplemented, и «update the core»
// пользователю полезнее, чем gRPC-строка про неизвестный метод.
func chainToggleErrorText(err error) string {
	if errors.Is(err, core.ErrChainToggleUnsupported) {
		return locale.T("This core cannot switch chain positions on the fly — update the core.")
	}
	return err.Error()
}

// chainPositionText — состав позиции: номер, тег и во что он резолвится.
//
// `now` показывается только когда отличается от тега: у обычного узла они
// совпадают, и вторая половина строки повторяла бы первую. У группы же это
// единственный способ увидеть, через кого реально идёт трафик, не открывая
// вложенные селекторы.
func chainPositionText(i int, pos core.ChainPositionInfo) string {
	text := fmt.Sprintf("  %d. %s", i+1, pos.Tag)
	if now := strings.TrimSpace(pos.Now); now != "" && now != pos.Tag {
		text += "  ● " + now
	}
	if pos.Transparent {
		text += "  · " + locale.T("collapsed")
	}
	// Выключенная позиция помечается отдельно от схлопнутой: причина разная
	// (воля пользователя против direct в конфиге), и лечится по-разному.
	if pos.Disabled {
		text += "  · " + locale.T("off")
	}
	return text
}

// probeChainLayers меряет префиксы цепочки последовательно.
//
// Именно последовательно: позиция i недостижима иначе как через i-1, и
// параллельный прогон поднимал бы туннели одновременно, искажая замеры друг
// друга.
func probeChainLayers(ac *core.AppController, info core.ChainInfo) []chainLayerResult {
	results := make([]chainLayerResult, len(info.Positions))
	for run := 0; run < chainProbeWarmUpRuns; run++ {
		for i, pos := range info.Positions {
			if pos.Transparent || pos.Disabled {
				// Мерить нечего: в схлопнутой позиции выбран direct, а
				// выключенная исключена из маршрута — её служебный тег
				// измерил бы путь БЕЗ неё, то есть чужую позицию.
				results[i] = chainLayerResult{Skipped: true}
				continue
			}
			delay, coreErr, err := ac.ProbeChainLayer(info.Tag, i)
			if err != nil {
				debuglog.WarnLog("chain probe: %s#%d: %v", info.Tag, i, err)
				results[i] = chainLayerResult{Error: err.Error()}
				continue
			}
			results[i] = chainLayerResult{DelayMs: delay, Error: coreErr}
		}
	}
	return results
}

// applyChainProbeResults раскладывает замеры по строкам.
func applyChainProbeResults(results []chainLayerResult, delays []*widget.Label, errLabel *widget.Label) {
	firstError := ""
	prev := int64(-1) // задержка предыдущей ИЗМЕРЕННОЙ позиции
	for i, res := range results {
		if i >= len(delays) {
			break
		}
		// Importance выставляется на КАЖДЫЙ замер, а не только при ошибке:
		// однажды покрасневшая позиция иначе рисовала бы опасным стилем и
		// все последующие успешные цифры.
		delays[i].Importance = widget.MediumImportance
		switch {
		case res.Skipped:
			delays[i].SetText(locale.T("—"))
			// Опорную точку схлопнутая позиция не сбрасывает: пакет через
			// неё проходит, просто без своего звена.
		case res.Error != "":
			delays[i].SetText(locale.T("error"))
			delays[i].Importance = widget.DangerImportance
			if firstError == "" {
				firstError = res.Error
			}
			// Следующая позиция теряет опорную точку: её цену не вычислить.
			prev = -1
		default:
			delays[i].SetText(chainDelayText(res.DelayMs, prev))
			prev = res.DelayMs
		}
		delays[i].Refresh()
	}
	if firstError != "" {
		errLabel.SetText(firstError)
		errLabel.Show()
	}
}

// chainDelayText — накопленное время и цена хопа.
//
// Накопленное отвечает «сколько всего до сюда», дельта — «кто добавил».
// Отрицательная дельта (сетевой шум, разные маршруты проб) показывается как
// «+0»: «этот хоп ускорил маршрут» — утверждение, из которого пользователю
// нечего извлечь.
func chainDelayText(delay, prev int64) string {
	if prev < 0 {
		return fmt.Sprintf("%d ms", delay)
	}
	cost := delay - prev
	if cost < 0 {
		cost = 0
	}
	return fmt.Sprintf("%d ms  (+%d)", delay, cost)
}
