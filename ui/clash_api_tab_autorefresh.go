package ui

import (
	"sync"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/platform"
)

// autoRefreshInterval — период тихого перечитывания GET /proxies/{group} на
// вкладке Remote.
//
// 5 секунд: список узлов удалённой машины меняется не сам по себе, а от
// действий на ней (переключение узла из другого клиента, рестарт ядра), и
// пятисекундная задержка такие правки показывает как «почти сразу». Чаще
// смысла нет — это сетевой round-trip до роутера на каждый тик.
const autoRefreshInterval = 5 * time.Second

// autoRefreshSteps — сколько шагов отсчёта тикер сообщает индикатору за цикл.
//
// Шаг — секунда, и на каждом значок ↻ становится чуть тусклее; на нуле идёт
// запрос и яркость возвращается разом (см. refreshPulse).
const autoRefreshSteps = int(autoRefreshInterval / time.Second)

// proxyAutoRefresh — тикер тихого обновления списка прокси одной панели.
//
// Только для Remote: локальное ядро своё, все изменения его состава проходят
// через сам лаунчер, и опрашивать себя по кругу незачем. У удалённой машины
// собеседник внешний — её узел могли переключить мимо нас.
//
// Тикер сознательно НЕ живёт всё время работы приложения. Он поднимается лишь
// когда одновременно верно: вкладка Remote активна, машина подключена, окно не
// свёрнуто и система не спит. Фоновые запросы к роутеру, которые никто не
// видит, — это разряд батареи и мусор в логах машины; при уходе на Local или в
// трей тикер останавливается, а не «пропускает такты».
type proxyAutoRefresh struct {
	mu      sync.Mutex
	stop    chan struct{}
	tick    func()
	enabled bool // вкладка активна
	visible bool // окно не свёрнуто

	// onCountdown — индикатор обратного отсчёта: сколько шагов осталось до
	// запроса (autoRefreshSteps..0). Вызывается раз в секунду, всегда с
	// UI-потока. nil — индикатора нет.
	//
	// Отсчёт ведёт сам тикер, а не отдельная анимация в UI: два независимых
	// таймера неизбежно разъезжаются, и значок гас бы не в такт запросам.
	onCountdown func(left int)
}

// SetTabActive сообщает, смотрит ли пользователь на эту вкладку.
func (r *proxyAutoRefresh) SetTabActive(on bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.enabled = on
	r.mu.Unlock()
	r.reconcile()
}

// SetWindowVisible сообщает, видно ли окно (не свёрнуто ли в трей).
func (r *proxyAutoRefresh) SetWindowVisible(on bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.visible = on
	r.mu.Unlock()
	r.reconcile()
}

// SetCountdownFunc вешает индикатор обратного отсчёта.
//
// Ставится до первого запуска тикера: колбэк захватывается горутиной на старте,
// и смена его на ходу до неё бы не доехала.
func (r *proxyAutoRefresh) SetCountdownFunc(f func(left int)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.onCountdown = f
	r.mu.Unlock()
}

// reconcile приводит состояние тикера к условиям: должен идти или нет.
func (r *proxyAutoRefresh) reconcile() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	want := r.enabled && r.visible && r.tick != nil
	running := r.stop != nil

	switch {
	case want && !running:
		stop := make(chan struct{})
		r.stop = stop
		tick := r.tick
		countdown := r.onCountdown
		go func() {
			// Шаг — секунда, а не весь интервал: тот же таймер и запускает
			// запрос, и гасит значок. Отдельный таймер на анимацию
			// разъезжался бы с запросами, и пульс шёл бы не в такт.
			t := time.NewTicker(time.Second)
			defer t.Stop()
			left := autoRefreshSteps
			if countdown != nil {
				countdown(left)
			}
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					left--
					if countdown != nil {
						countdown(left)
					}
					if left > 0 {
						continue
					}
					tick()
					left = autoRefreshSteps
					if countdown != nil {
						countdown(left)
					}
				}
			}
		}()
	case !want && running:
		close(r.stop)
		r.stop = nil
		// Гасим индикатор: иначе значок замер бы полупотухшим, обещая
		// обновление, которого уже не будет (ушли на Local, свернули окно).
		if r.onCountdown != nil {
			r.onCountdown(-1)
		}
	}
}

// Stop глушит тикер насовсем (закрытие окна).
func (r *proxyAutoRefresh) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stop != nil {
		close(r.stop)
		r.stop = nil
	}
}

// startAutoRefresh привязывает к панели тихое обновление списка.
//
// silent — тот же запрос, что и по кнопке ↻, но без побочных эффектов ручного
// действия: не трогает скролл, статус-строку и выделение (см. комментарий в
// silentRefreshProxies). Пользователь не должен замечать тик иначе как по
// изменившимся данным.
func (p *ProxyListPanel) startAutoRefresh(ac *core.AppController) {
	if p == nil || p.scope != services.ScopeRemote {
		return
	}
	p.autoRefresh = &proxyAutoRefresh{}
	p.autoRefresh.tick = func() {
		// Машина могла отключиться между тиками: запрос ушёл бы с пустой
		// группой и вернул «group "" not found» — ошибку на ровном месте.
		if _, _, connected := GetLxdRemoteOverride(); !connected {
			return
		}
		if platform.IsSleeping() {
			return
		}
		if p.silentRefresh != nil {
			p.silentRefresh(ac)
		}
	}
}

// silentRefreshProxies перечитывает список узлов, не трогая то, чем пользователь
// сейчас управляет.
//
// Отличия от ручного Refresh принципиальны для тикера раз в 5 секунд:
//   - НЕТ ScrollToTop: иначе список прыгал бы в начало каждые пять секунд, и
//     пролистать его до конца стало бы невозможно.
//   - НЕТ «Загружаю…» в статус-строке: она мигала бы вечно, а осмысленный
//     текст («узлов: N») пользователь прочитать не успевал бы.
//   - НЕТ пересортировки под курсором: порядок берётся прежний, иначе строка
//     уезжает из-под клика между наведением и нажатием.
//
// Пинги сохраняются так же, как в ручном пути: они локальные, машина о них
// ничего не знает и в ответе их нет.
func silentRefreshProxies(ac *core.AppController, scope services.ProxyScope, group string) {
	if ac == nil || ac.APIService == nil || group == "" {
		return
	}
	if _, _, enabled, _ := EffectiveClashAPIConfigIn(ac, scope); !enabled {
		return
	}
	gen := CurrentGeneration()
	transport := EffectiveProxyTransportIn(ac, scope)
	proxies, now, err := transport.GroupProxies(group)
	if err != nil {
		// Тихий путь — тихая ошибка: всплывать диалогом раз в пять секунд
		// из-за моргнувшей сети нельзя. Ручной Refresh покажет её честно.
		debuglog.WarnLog("clash_api_tab: auto-refresh failed: %v", err)
		return
	}
	fyne.Do(func() {
		if gen != CurrentGeneration() {
			return
		}
		if _, _, connected := GetLxdRemoteOverride(); !connected {
			return
		}
		ac.SetProxiesList(mergePreservingOrder(ac.GetProxiesList(), proxies))
		ac.SetActiveProxyName(now)
		if ac.UIService != nil && ac.UIService.ProxiesListWidget != nil {
			ac.UIService.ProxiesListWidget.Refresh()
		}
	})
}

// refreshPulse — кнопка ↻, которая своим цветом показывает авто-обновление.
//
// Один элемент вместо двух: это и есть кнопка перечитывания (тот же клик, та
// же подсказка), и одновременно индикатор. Отдельный счётчик рядом со строкой
// мельтешил бы и сообщал то, чем пользователь всё равно не пользуется, —
// сколько секунд осталось. Здесь значок просто «дышит»: плавно тускнеет к
// моменту запроса и разом возвращается в полную яркость, когда данные
// перечитаны. Такое движение читается боковым зрением и не тянет на себя
// внимание.
//
// canvas.Text, а не Button с иконкой: цвет текста меняется одним полем, тогда
// как у кнопки нет ни цвета, ни прозрачности — пришлось бы подменять её
// картинкой.
type refreshPulse struct {
	text    *canvas.Text
	anim    *fyne.Animation
	content fyne.CanvasObject
}

// refreshPulseGlyph — тот же символ, что и в иконке темы (Material «refresh»):
// круговая стрелка с разрывом справа вверху.
const refreshPulseGlyph = "↻"

func newRefreshPulse(tapped func(), tooltip string) *refreshPulse {
	t := canvas.NewText(refreshPulseGlyph, theme.Color(theme.ColorNameForeground))
	// Крупнее текста списка: это орган управления, а не подпись, и мелкий
	// символ было бы не попасть мышью.
	t.TextSize = theme.TextSize() * 1.4
	t.Alignment = fyne.TextAlignCenter

	p := &refreshPulse{text: t}

	// Ширина под кнопку: голый текст занял бы ровно свои несколько пикселей, и
	// попасть по нему мышью было бы трудно. Заодно место в строке остаётся
	// постоянным, как было у прежней кнопки.
	size := fyne.NewSize(theme.IconInlineSize()*2, theme.IconInlineSize()*2)
	boxed := container.New(layout.NewGridWrapLayout(size), container.NewCenter(t))

	// TapWrap сам ставит курсор-указатель, так что кликабельность видна и без
	// подсказки; тултипа у него нет — оборачивать ради этого в кнопку значило
	// бы вернуть рамку, от которой мы и уходим.
	_ = tooltip
	p.content = fynewidget.NewTapWrap(boxed, tapped)
	return p
}

// Object — готовый к вставке в строку виджет.
func (p *refreshPulse) Object() fyne.CanvasObject {
	if p == nil {
		return nil
	}
	return p.content
}

// pulseColors — края «дыхания»: от приглушённого к обычному цвету текста.
//
// Тусклый край — Disabled, а не прозрачность: он уже подобран темой под оба
// оформления, светлое и тёмное, и значок не пропадает на фоне ни в одном.
func pulseColors() (dim, bright color.Color) {
	return theme.Color(theme.ColorNameDisabled), theme.Color(theme.ColorNameForeground)
}

// SetCountdown принимает шаги тикера: autoRefreshSteps — запрос только что
// прошёл, 0 — идёт сейчас, отрицательное — тикер остановлен.
//
// Плавность между секундными шагами даёт анимация Fyne, а не свой таймер:
// шагов всего пять, и без интерполяции «дыхание» выглядело бы ступенчатым.
func (p *refreshPulse) SetCountdown(left int) {
	if p == nil || p.text == nil {
		return
	}
	fyne.Do(func() {
		if p.anim != nil {
			p.anim.Stop()
			p.anim = nil
		}
		dim, bright := pulseColors()

		// Тикер остановлен — значок обычного цвета: он остаётся рабочей
		// кнопкой, даже когда авто-обновления нет.
		if left < 0 {
			p.text.Color = bright
			p.text.Refresh()
			return
		}

		// Запрос прошёл (полный отсчёт впереди) — возвращаем яркость разом:
		// это и есть тот момент, ради которого индикатор существует.
		if left >= autoRefreshSteps {
			p.text.Color = bright
			p.text.Refresh()
			return
		}

		// Между шагами — плавное угасание на одну секунду вперёд.
		from := stepColor(left+1, dim, bright)
		to := stepColor(left, dim, bright)
		p.anim = canvas.NewColorRGBAAnimation(from, to, time.Second, func(c color.Color) {
			p.text.Color = c
			p.text.Refresh()
		})
		p.anim.Start()
	})
}

// stepColor — цвет значка на шаге left: at full steps — яркий, на нуле —
// самый тусклый.
func stepColor(left int, dim, bright color.Color) color.Color {
	if autoRefreshSteps <= 0 {
		return bright
	}
	if left < 0 {
		left = 0
	}
	if left > autoRefreshSteps {
		left = autoRefreshSteps
	}
	return blendColor(dim, bright, float64(left)/float64(autoRefreshSteps))
}

// blendColor смешивает два цвета: k=0 → dim, k=1 → bright.
func blendColor(dim, bright color.Color, k float64) color.Color {
	dr, dg, db, da := dim.RGBA()
	br, bg, bb, ba := bright.RGBA()
	mix := func(d, b uint32) uint8 {
		v := float64(d>>8)*(1-k) + float64(b>>8)*k
		return uint8(v + 0.5)
	}
	return color.NRGBA{R: mix(dr, br), G: mix(dg, bg), B: mix(db, bb), A: mix(da, ba)}
}
