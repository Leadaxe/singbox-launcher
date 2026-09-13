// File servers_node_info_chain.go — секция цепочки в окне «Info» (SPEC 110).
//
// Почему здесь, а не в редакторе цепочки: редактор правит КОНФИГ, а его
// правки доезжают до ядра только через сохранение и пересборку. Замерять
// там значило бы показывать цифры прежнего маршрута рядом с изменённым
// списком позиций — правдоподобные и не про то, что на экране. Здесь же
// строка соответствует работающему ядру по построению.
//
// SPEC 124: у каждой позиции своя строка ошибки и хвост состояния звена
// словами ядра (`starting | active | idle`); общая строка под списком
// остаётся за тем, что к позиции не привязано (старое ядро, отвалившийся
// RPC).
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
	// Transport — ошибка не ядра, а дороги до него (RPC отвалился, режим
	// не тот). К позиции она не относится и показывается общей строкой.
	Transport string
	// Skipped — позиция схлопнута (direct) или выключена, мерить нечего.
	Skipped bool
}

// chainRows — виджеты секции, которые обновляются по состоянию ядра.
//
// Один тип вместо четырёх параллельных срезов: их индексы обязаны
// совпадать, и передавать их по отдельности значило бы проверять это в
// каждой функции.
type chainRows struct {
	text    []*widget.Label // состав позиции
	delay   []*widget.Label // замер, правый столбец
	toggle  []*widget.Check // тумблер позиции
	posErr  []*widget.Label // ошибка ЭТОЙ позиции, под её строкой
	general *widget.Label   // ошибка, не привязанная к позиции
}

func (r *chainRows) clearDelays() {
	for _, d := range r.delay {
		d.SetText("")
	}
}

// setPosErr пишет текст в строку позиции; пустой текст прячет её.
func (r *chainRows) setPosErr(i int, text string) {
	if i < 0 || i >= len(r.posErr) {
		return
	}
	text = strings.TrimSpace(text)
	r.posErr[i].SetText(text)
	if text == "" {
		r.posErr[i].Hide()
	} else {
		r.posErr[i].Show()
	}
}

func (r *chainRows) setGeneral(text string) {
	text = strings.TrimSpace(text)
	r.general.SetText(text)
	if text == "" {
		r.general.Hide()
	} else {
		r.general.Show()
	}
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

// newChainErrLabel — красная строка с переносом. Перенос обязателен:
// сообщение ядра длинное, а Label без Wrapping задаёт окну минимальную
// ширину и раздувает его на весь экран.
func newChainErrLabel() *widget.Label {
	l := widget.NewLabel("")
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.DangerImportance
	l.Hide()
	return l
}

// buildChainSection рисует позиции и кнопку замера.
func buildChainSection(ac *core.AppController, box *fyne.Container, win fyne.Window, info core.ChainInfo) {
	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.Tf("Chain positions (%d)", len(info.Positions))))

	n := len(info.Positions)
	rows := &chainRows{
		text:    make([]*widget.Label, n),
		delay:   make([]*widget.Label, n),
		toggle:  make([]*widget.Check, n),
		posErr:  make([]*widget.Label, n),
		general: newChainErrLabel(),
	}
	// applying — идёт программная расстановка галочек, а не клик
	// пользователя. Fyne зовёт OnChanged и на SetChecked, и отличить одно
	// от другого больше нечем.
	applying := false

	// Строка на позицию: слева тумблер, затем состав, справа замер. Галочка
	// слева читается как «хоп участвует в маршруте» — список позиций и есть
	// маршрут; правый край остаётся за задержками, иначе тумблер и цифра
	// дрались бы за одно место. Под строкой — её ошибка, скрытая пока пуста.
	for i := range info.Positions {
		rows.text[i] = widget.NewLabel("")
		rows.text[i].Truncation = fyne.TextTruncateEllipsis

		rows.delay[i] = widget.NewLabel("")
		rows.delay[i].Alignment = fyne.TextAlignTrailing

		rows.posErr[i] = newChainErrLabel()

		rows.toggle[i] = newChainPositionToggle(ac, info.Tag, i, &applying, rows, func(fresh core.ChainInfo) {
			// Перерисовываем ВСЮ секцию, а не одну строку: маршрут общий,
			// и выключенный хоп меняет то, во что резолвятся соседние
			// позиции (группа выше могла держать выбор через него).
			applyChainRows(fresh, rows, &applying)
		})

		box.Add(container.NewBorder(nil, nil, rows.toggle[i], rows.delay[i], rows.text[i]))
		box.Add(rows.posErr[i])
	}
	// Начальное состояние ставим ПОСЛЕ создания тумблеров и под флагом:
	// SetChecked дёргает OnChanged, и без него отрисовка окна отправила бы
	// ядру тумблер, которого пользователь не нажимал.
	applyChainRows(info, rows, &applying)
	box.Add(rows.general)

	var probeBtn *widget.Button
	probeBtn = widget.NewButtonWithIcon(
		locale.T("Probe by position"), theme.ViewRefreshIcon(), func() {
			probeBtn.Disable()
			probeBtn.SetText(locale.T("Measuring…"))
			rows.clearDelays()
			// Проба чистит ВСЕ строки позиций: она заново проверяет каждую,
			// и старая ошибка прогрева рядом со свежим замером путала бы.
			for i := range rows.posErr {
				rows.setPosErr(i, "")
			}
			rows.setGeneral("")

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
					// Замеры раскладываем ПЕРЕД составом: ошибка пробы
					// свежее last_error звена, и заполнять пустые строки
					// состав должен после неё.
					applyChainProbeResults(results, rows)
					// Строки состава тоже обновляем: если выбор группы
					// сменился, показать старый тег рядом со свежей
					// задержкой значило бы соврать вдвойне.
					applyChainRows(cur, rows, &applying)
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
	rows *chainRows,
	refresh func(core.ChainInfo),
) *widget.Check {
	var check *widget.Check
	check = widget.NewCheck("", func(enabled bool) {
		if *applying {
			return
		}
		check.Disable()
		// Чистим СВОЮ строку и общую: прежняя ошибка этой позиции к новому
		// клику не относится, чужие строки — не наше дело.
		rows.setPosErr(pos, "")
		rows.setGeneral("")
		// Все замеры протухли разом: путь через позицию i входит в цену
		// каждой позиции выше. Цифра прежнего маршрута рядом с новым
		// составом врала бы, и заметить это было бы нечем.
		rows.clearDelays()

		go func() {
			warmupErr, err := ac.SetChainPositionEnabled(chainTag, pos, enabled)
			// Состояние перечитываем здесь же, в фоне: ChainFor — это RPC к
			// ядру, и его дедлайн в UI-потоке подвесил бы окно целиком.
			fresh, ok := ac.ChainFor(chainTag)
			fyne.Do(func() {
				defer check.Enable()
				switch {
				case err != nil:
					// Отказ дороги или ядра целиком — к позиции не привязан,
					// идёт общей строкой.
					debuglog.WarnLog("chain toggle: %s#%d enabled=%v: %v", chainTag, pos, enabled, err)
					rows.setGeneral(chainToggleErrorText(err))
					if chainToggleNeedsRevert(err, warmupErr, ok) {
						// Двойной сбой: ядро переключение отвергло И состав
						// перечитать не удалось — приводить галочку не по чему,
						// а оставить её нажатой значит соврать про состояние
						// ядра. Возвращаем ровно то положение, что было до
						// клика (SPEC 113-E).
						//
						// Под applying: SetChecked зовёт OnChanged, и без флага
						// откат сам ушёл бы в ядро обратным тумблером.
						*applying = true
						check.SetChecked(!enabled)
						*applying = false
					}
				case strings.TrimSpace(warmupErr) != "":
					// Флаг ядро применило, а звено не поднялось. Это диагноз
					// узла, а не отказ переключения: галочка остаётся там,
					// куда её поставил пользователь, текст под ЕЁ строкой
					// объясняет, почему трафик через позицию пока не пойдёт.
					// Пишется ДО refresh: last_error звена из состава старее
					// и пустую строку не перекроет, а занятую не трогает.
					debuglog.WarnLog("chain toggle warmup: %s#%d: %s", chainTag, pos, warmupErr)
					rows.setPosErr(pos, warmupErr)
				}
				if ok {
					refresh(fresh)
				}
			})
		}()
	})
	return check
}

// chainToggleNeedsRevert — надо ли самим вернуть галочку в пред-кликовое
// положение (SPEC 113-E).
//
// Три исхода клика, и откат нужен ровно в одном:
//   - ядро приняло переключение (err == nil) — галочка права, а состав всё
//     равно приедет из refresh;
//   - ядро отвергло, но состав перечитан (refreshed) — галочку приведёт к
//     состоянию ядра applyChainRows, и второй источник правды тут вреден;
//   - ядро отвергло И состав перечитать не удалось — приводить не по чему,
//     а оставленная нажатой галочка врёт про состояние ядра.
//
// warmupErr к откату отношения не имеет: флаг ядро применило, не поднялось
// само звено — это диагноз узла, и галочка остаётся там, куда её поставили.
func chainToggleNeedsRevert(err error, warmupErr string, refreshed bool) bool {
	_ = warmupErr
	return err != nil && !refreshed
}

// applyChainRows приводит строки к состоянию, прочитанному у ядра.
// Состояние уже на руках — сюда попадаем из UI-потока, ходить в ядро
// отсюда нельзя.
//
// Под флагом applying: SetChecked зовёт OnChanged, и без него приведение к
// состоянию ядра само отправило бы ядру новый тумблер — рекурсией.
//
// last_error звена заполняет только ПУСТЫЕ строки: то, что уже стоит
// (прогрев после клика, ошибка пробы), свежее состава, который ядро могло
// ещё не обновить.
func applyChainRows(fresh core.ChainInfo, rows *chainRows, applying *bool) {
	*applying = true
	defer func() { *applying = false }()
	for i := range rows.text {
		if i >= len(fresh.Positions) {
			break
		}
		pos := fresh.Positions[i]
		rows.text[i].SetText(chainPositionText(i, pos))
		rows.toggle[i].SetChecked(!pos.Disabled)
		if rows.posErr[i].Text == "" {
			rows.setPosErr(i, pos.LastError)
		}
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

// chainPositionText — состав позиции: номер, тег, во что он резолвится и
// состояние звена.
//
// `now` показывается только когда отличается от тега: у обычного узла они
// совпадают, и вторая половина строки повторяла бы первую. У группы же это
// единственный способ увидеть, через кого реально идёт трафик, не открывая
// вложенные селекторы.
//
// Состояние звена — словами ядра (`starting | active | idle`), своих не
// выдумываем. У выключенной позиции оно остаётся рядом с «off»: ядро не
// рвёт звено принудительно, его забирает idle-эвикшн, и «off · active»
// честно говорит, что звено ещё держит соединения. Пустое состояние — вход
// (не клонируется) или звено ещё не создано (urltest-позиция рождает его
// лениво); хвоста тогда нет.
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
	if state := chainCloneStateText(pos.CloneState); state != "" {
		text += "  · " + state
	}
	return text
}

// chainCloneStateText — перевод состояния звена. Неизвестное слово ядра
// показывается как есть: новая версия ядра может добавить состояние, и
// прятать его хуже, чем показать без перевода.
func chainCloneStateText(state string) string {
	switch strings.TrimSpace(state) {
	case "":
		return ""
	case "starting":
		return locale.T("starting")
	case "active":
		return locale.T("active")
	case "idle":
		return locale.T("idle")
	default:
		return strings.TrimSpace(state)
	}
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
				results[i] = chainLayerResult{Transport: err.Error()}
				continue
			}
			results[i] = chainLayerResult{DelayMs: delay, Error: coreErr}
		}
	}
	return results
}

// applyChainProbeResults раскладывает замеры по строкам.
//
// Ошибка ядра ложится под СВОЮ позицию; ошибка дороги — в общую строку,
// первая из встреченных: если RPC отвалился, он отвалился для всех.
func applyChainProbeResults(results []chainLayerResult, rows *chainRows) {
	transport := ""
	prev := int64(-1) // задержка предыдущей ИЗМЕРЕННОЙ позиции
	for i, res := range results {
		if i >= len(rows.delay) {
			break
		}
		d := rows.delay[i]
		// Importance выставляется на КАЖДЫЙ замер, а не только при ошибке:
		// однажды покрасневшая позиция иначе рисовала бы опасным стилем и
		// все последующие успешные цифры.
		d.Importance = widget.MediumImportance
		switch {
		case res.Skipped:
			d.SetText(locale.T("—"))
			// Опорную точку схлопнутая позиция не сбрасывает: пакет через
			// неё проходит, просто без своего звена.
		case res.Transport != "":
			d.SetText(locale.T("error"))
			d.Importance = widget.DangerImportance
			if transport == "" {
				transport = res.Transport
			}
			prev = -1
		case res.Error != "":
			d.SetText(locale.T("error"))
			d.Importance = widget.DangerImportance
			rows.setPosErr(i, res.Error)
			// Следующая позиция теряет опорную точку: её цену не вычислить.
			prev = -1
		default:
			d.SetText(chainDelayText(res.DelayMs, prev))
			prev = res.DelayMs
		}
		d.Refresh()
	}
	if transport != "" {
		rows.setGeneral(transport)
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
