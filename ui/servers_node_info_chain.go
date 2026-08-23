// File servers_node_info_chain.go — секция цепочки в окне «Info» (SPEC 110).
//
// Почему здесь, а не в редакторе цепочки: редактор правит КОНФИГ, а его
// правки доезжают до ядра только через сохранение и пересборку. Замерять
// там значило бы показывать цифры прежнего маршрута рядом с изменённым
// списком позиций — правдоподобные и не про то, что на экране. Здесь же
// строка соответствует работающему ядру по построению.
package ui

import (
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
	box.Add(sectionHeader(locale.Tf("servers.node_info_section_chain", len(info.Positions))))

	// Строка на позицию: слева состав, справа замер. Обе колонки живут в
	// одной строке, чтобы задержка читалась напротив своего хопа, а не
	// отдельным списком, который пришлось бы сопоставлять глазами.
	rows := make([]*widget.Label, len(info.Positions))
	delays := make([]*widget.Label, len(info.Positions))
	for i, pos := range info.Positions {
		rows[i] = widget.NewLabel(chainPositionText(i, pos))
		rows[i].Truncation = fyne.TextTruncateEllipsis

		delays[i] = widget.NewLabel("")
		delays[i].Alignment = fyne.TextAlignTrailing

		box.Add(container.NewBorder(nil, nil, nil, delays[i], rows[i]))
	}

	// Текст ошибки — под списком, с переносом: сообщение ядра длинное и
	// в колонку задержки не помещается.
	errLabel := widget.NewLabel("")
	errLabel.Wrapping = fyne.TextWrapWord
	errLabel.Importance = widget.DangerImportance
	errLabel.Hide()
	box.Add(errLabel)

	var probeBtn *widget.Button
	probeBtn = widget.NewButtonWithIcon(
		locale.T("servers.node_info_chain_probe"), theme.ViewRefreshIcon(), func() {
			probeBtn.Disable()
			probeBtn.SetText(locale.T("servers.node_info_chain_probing"))
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
					probeBtn.SetText(locale.T("servers.node_info_chain_probe_again"))
				})
			}()
		})
	box.Add(container.NewCenter(probeBtn))
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
		text += "  · " + locale.T("servers.node_info_chain_transparent")
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
			if pos.Transparent {
				// Схлопнутая позиция: в ней выбран direct, звена нет.
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
		switch {
		case res.Skipped:
			delays[i].SetText(locale.T("servers.node_info_chain_skipped"))
			// Опорную точку схлопнутая позиция не сбрасывает: пакет через
			// неё проходит, просто без своего звена.
		case res.Error != "":
			delays[i].SetText(locale.T("servers.node_info_chain_failed"))
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
