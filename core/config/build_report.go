// File build_report.go — типизированный отчёт попытки сборки (SPEC 115 §2).
//
// Расширение реестра исключений (SPEC 112-B / 113-B), а НЕ второе хранилище
// рядом с ним: исключение источника — частный случай записи отчёта, и держать
// два независимых реестра значило бы получить два разных ответа на вопрос
// «что случилось на последней сборке». Поэтому excluded_sources_registry.go
// теперь тонкий фасад поверх этого файла, а его прежний контракт
// (SetExcludedSources переписывает, AppendExcludedSources доливает,
// ExcludedSourceReason ищет по ULID) сохранён дословно.
//
// Зачем в памяти, а не в state.json — довод прежний: запись отчёта живёт
// ровно столько, сколько живёт её причина. Провайдер вернул хоп — записи нет.
// Сохранённая на диск, она пережила бы собственную причину и врала бы
// пользователю до следующей сборки.
//
// Инвалидация (SPEC 115 §2): любая правка модели Мастера сбрасывает отчёт
// целиком. Раскраска источников и окно «Итога» обязаны молчать, а не
// показывать итог сборки, сделанной ДО правки: пользователь чинит источник и
// смотрит, ушла ли пометка, — устаревшая пометка отвечает на этот вопрос
// неправильно.
//
// Читатели ходят сюда из горутин, отсюда мьютекс; сами виджеты трогать только
// через fyne.Do — это забота вызывающего.
package config

import "sync"

// BuildReportKind — вид записи отчёта (SPEC 115 §2).
//
// Строковые константы, а не iota: вид уезжает в лог и в текст отчёта, и
// читаемое имя там полезнее номера.
type BuildReportKind string

const (
	// BuildReportSourceExcluded — источник выпал из конфига ЦЕЛИКОМ:
	// detour-хоп не разрешился, источник ссылки удалён, кольцо ссылок.
	BuildReportSourceExcluded BuildReportKind = "source_excluded"

	// BuildReportNodesDropped — из источника сняты отдельные узлы (последний
	// рубеж, граф-санитайзер). Группируется по источнику: у подписки на 500
	// узлов один сломанный переход снимает их все разом, и 500 одинаковых
	// записей были бы не сообщением, а шумом.
	BuildReportNodesDropped BuildReportKind = "nodes_dropped"

	// BuildReportChainFailed — источник-цепочка не стал узлом (SPEC 110).
	BuildReportChainFailed BuildReportKind = "chain_failed"

	// BuildReportNaiveDegraded — naive-узлы сняты, потому что ядро их не
	// умеет (SPEC 044 feature-probe).
	BuildReportNaiveDegraded BuildReportKind = "naive_degraded"

	// BuildReportSourceParseFailed — источник не дал конфигу НИ ОДНОГО узла:
	// не фетчнулся, или фетчнулся и разобрался в ноль (SPEC 115).
	//
	// Отдельный вид от source_excluded, хотя строка Sources у обоих несёт ⚠:
	// исключённый источник узлы дал, но выпал из конфига из-за ссылки, и чинить
	// надо ссылку; этот не дал ничего, и чинить надо саму подписку. Показать
	// второе первым значило бы отправить пользователя искать несуществующий
	// сломанный detour.
	//
	// Причина — компактная (первые несколько РАЗНЫХ причин отбраковки), а не
	// стенограмма: у подписки на 500 узлов причина одна, повторённая 500 раз.
	BuildReportSourceParseFailed BuildReportKind = "source_parse_failed"

	// BuildReportTargetMissing — цель detour не существует в собранном
	// конфиге. Отдельный вид от nodes_dropped: там субъект — источник и
	// счётчик узлов, здесь — сама несуществующая цель.
	BuildReportTargetMissing BuildReportKind = "target_missing"

	// BuildReportFetchDegraded — деградация ПОСЛЕДНЕГО ОБНОВЛЕНИЯ подписки,
	// прочитанная из состояния (SPEC 118 W4, Т3/Р8): битая запись тела,
	// потерянный член группы, обрезка капом, недостоверный ответ.
	//
	// Отдельный вид от source_parse_failed: тот про источник, не давший НИ
	// ОДНОГО узла (чинить подписку целиком), этот — про частичную потерю у
	// источника, который узлы дал. Раньше такие строки рождались parse-стадией
	// сборки; теперь разбор живёт только в fetch и пишет их в
	// `update_status.warnings`, а отчёт читает их ОТТУДА — синхронно, без
	// пересборки (Р8: «источник с битой записью виден в Итоге после fetch»).
	BuildReportFetchDegraded BuildReportKind = "fetch_degraded"

	// BuildReportEmitDegraded — деградация ЭМИССИИ из материализованных
	// nodes[] (SPEC 118 W4): нерезолвнутая позиция цепочки, выпавший член
	// Auto-группы, снятое умолчание, столкновение тегов в гарде.
	//
	// Отдельный вид от fetch_degraded: тот про то, что провайдер прислал,
	// этот — про то, что лаунчер не смог собрать из уже принятого.
	BuildReportEmitDegraded BuildReportKind = "emit_degraded"
)

// BuildReportEntry — одна запись отчёта.
//
// Subject — О ЧЁМ запись человеческими словами: имя источника, имя цепочки,
// тег отсутствующей цели. Reason — ПОЧЕМУ, по стандарту SPEC 112-A: причина
// называет обе стороны и не светит ULID, когда известно человеческое имя.
//
// SourceID пуст у записей, которые не привязаны к источнику состояния (конфиг
// собран не из state.json, узел из ручного outbound шаблона). Такая запись
// остаётся годной для отчёта, но раскрасить ею строку Sources не к чему —
// именно поэтому поиск по пустому ULID никогда не совпадает.
type BuildReportEntry struct {
	Kind        BuildReportKind
	Subject     string
	SourceID    string
	SourceLabel string
	Reason      string
	// NodeCount — сколько узлов снято (только BuildReportNodesDropped).
	// Ноль у остальных видов: там считать нечего.
	NodeCount int
}

// BuildGeneration — номер попытки сборки.
//
// Реестр один на процесс, а писателей у него несколько и работают они
// параллельно: фоновое авто-обновление подписок, разбор Мастера, сборка
// вкладки «Итог». Мьютекс защищает поля, но не порядок: без номера попытки
// поздняя запись чужой, уже брошенной сборки просто дописывалась бы в
// текущий отчёт, а её FinishBuildReport объявлял бы готовым чужой итог.
// Номер делает попытку опознаваемой — Feed/Finish с устаревшим номером
// игнорируются, а гейт Save спрашивает про ТУ попытку, чей отчёт показан.
//
// Ноль — «никакой попытки»: с ним не совпадает ни одна живая.
type BuildGeneration uint64

var (
	buildReportMu sync.RWMutex
	buildReport   []BuildReportEntry
	// buildReportReady — БЫЛА ли попытка сборки после последней инвалидации.
	//
	// Отдельный флаг, а не «len(buildReport) == 0»: чистая сборка и «сборки
	// ещё не было» — разные состояния с разным поведением UI. Первое даёт
	// «предупреждений нет» и открытую кнопку Save, второе — прелоадер и
	// закрытую. Схлопнув их в пустой слайс, мы бы разрешили Save до сборки.
	buildReportReady bool
	// buildReportGen — номер попытки, чьи записи сейчас лежат в отчёте.
	buildReportGen BuildGeneration
)

// ResetBuildReport стирает отчёт и снимает признак состоявшейся сборки.
//
// Зовётся при ЛЮБОЙ правке модели Мастера (SPEC 115 §2, инвалидация): после
// правки прежний отчёт описывает уже не ту конфигурацию.
//
// Номер попытки при этом СДВИГАЕТСЯ: идущая сейчас сборка после правки
// описывает уже не ту конфигурацию, и её запоздалый Finish не должен объявить
// отчёт готовым.
func ResetBuildReport() {
	buildReportMu.Lock()
	defer buildReportMu.Unlock()
	buildReport = nil
	buildReportReady = false
	buildReportGen++
}

// StartBuildReport открывает новую попытку сборки: прежние записи стираются,
// признак состоявшейся сборки НЕ выставляется (его ставит FinishBuildReport).
// Возвращает номер открытой попытки — с ним ходят Feed и Finish.
//
// Одна попытка — один отчёт (SPEC 115 §2): дозапись поверх прошлой попытки
// смешала бы причины двух разных сборок.
func StartBuildReport() BuildGeneration {
	buildReportMu.Lock()
	defer buildReportMu.Unlock()
	buildReport = nil
	buildReportReady = false
	buildReportGen++
	return buildReportGen
}

// FinishBuildReport помечает попытку состоявшейся: отчёт готов к показу и
// кнопка Save вправе открыться.
//
// Зовётся ТОЛЬКО на успешном завершении конвейера, и только со СВОИМ номером
// попытки: пока сборка шла, её мог обогнать другой писатель, и чужой отчёт
// объявлять готовым нельзя. Упавшая сборка Finish не зовёт вовсе — вместо
// отчёта показывается текст ошибки, и Save остаётся закрытой.
//
// Возвращает false, если попытка устарела (её обогнали или модель правили).
func FinishBuildReport(gen BuildGeneration) bool {
	buildReportMu.Lock()
	defer buildReportMu.Unlock()
	if gen != buildReportGen {
		return false
	}
	buildReportReady = true
	return true
}

// AddBuildReportEntries доливает записи в отчёт попытки gen.
//
// Записи чужой (обогнанной или инвалидированной) попытки отбрасываются: они
// описывают не ту конфигурацию, которую отчёт сейчас представляет.
//
// Дубликаты (тот же вид + тот же источник + тот же субъект) игнорируются:
// поставщиков у отчёта несколько и работают они в разное время — парсер знает
// про недоступный хоп сразу, последний рубеж узнаёт про исчезнувший селектор
// шаблона только когда сложен полный набор финальных тегов. Первая причина
// ближе к корню, поэтому побеждает она.
func AddBuildReportEntries(gen BuildGeneration, list []BuildReportEntry) {
	if len(list) == 0 {
		return
	}
	buildReportMu.Lock()
	defer buildReportMu.Unlock()
	if gen != buildReportGen {
		return
	}
	for _, e := range list {
		if buildReportHasLocked(e) {
			continue
		}
		buildReport = append(buildReport, e)
	}
}

// buildReportHasLocked — есть ли уже такая запись. Вызывать под замком.
func buildReportHasLocked(e BuildReportEntry) bool {
	for _, have := range buildReport {
		if have.Kind == e.Kind && have.SourceID == e.SourceID &&
			have.SourceLabel == e.SourceLabel && have.Subject == e.Subject {
			return true
		}
	}
	return false
}

// BuildReport — копия отчёта последней попытки, признак «попытка была» и номер
// этой попытки.
//
// Номер отдаётся вместе с записями, а не отдельным вызовом: читатель, взявший
// их по очереди, получил бы записи одной попытки и номер другой — ровно та
// рассинхронизация, ради которой номер и заведён. Гейт Save запоминает номер
// показанного отчёта и позже сверяет его через BuildReportReadyFor.
//
// Копия, а не сам слайс: читатель из UI не должен уметь испортить отчёт
// следующей сборке.
func BuildReport() ([]BuildReportEntry, bool, BuildGeneration) {
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	if len(buildReport) == 0 {
		return nil, buildReportReady, buildReportGen
	}
	return append([]BuildReportEntry(nil), buildReport...), buildReportReady, buildReportGen
}

// BuildReportReady — состоялась ли сборка после последней инвалидации.
//
// Гейт раскраски Sources спрашивает именно так: до первой попытки и после
// правки модели показывать нечего. Гейту Save этого мало — ему нужен ещё и
// номер попытки (BuildReportReadyFor).
func BuildReportReady() bool {
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	return buildReportReady
}

// BuildReportGenerationLive — жива ли попытка gen, то есть лежат ли в реестре
// ЕЁ записи.
//
// Спрашивает вторая половина конвейера, прежде чем доливать свои: между
// половинами попытку мог сбросить кто угодно — правка модели или другой
// писатель реестра. Доливка в мёртвую попытку молча пропадёт, а отчёт
// останется с одной половиной причин, объявленной полной.
func BuildReportGenerationLive(gen BuildGeneration) bool {
	if gen == 0 {
		return false
	}
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	return gen == buildReportGen
}

// BuildReportReadyFor — готов ли отчёт ИМЕННО той попытки, чей итог показан.
//
// Гейт кнопки Save спрашивает так, а не через BuildReportReady: пока
// пользователь читал отчёт, реестр мог перехватить другой писатель (фоновое
// авто-обновление подписок), и «готов» относилось бы уже к чужой сборке.
func BuildReportReadyFor(gen BuildGeneration) bool {
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	return buildReportReady && gen != 0 && gen == buildReportGen
}

// ParseFailedSourceReason — почему источник с данным ULID не дал конфигу ни
// одного узла; пусто, если узлы он дал.
//
// Спрашивает строка Wizard → Sources — тем же способом, что и про исключение:
// у неё на руках ULID, а не позиция в сборке. Пустой sourceID никогда не
// совпадает: записи без id (конфиг собран не из состояния) привязать к строке
// не к чему.
func ParseFailedSourceReason(sourceID string) string {
	if sourceID == "" {
		return ""
	}
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	for _, e := range buildReport {
		if e.Kind == BuildReportSourceParseFailed && e.SourceID == sourceID {
			return e.Reason
		}
	}
	return ""
}

// EmitWarningsForSource — деградации ЭМИССИИ, адресованные этому источнику
// (SPEC 116 W12, фикс 3).
//
// До этой волны эмиссионные записи адресата не имели вовсе (субъект
// "emission"), и строка Sources молчала: пользователь видел «узел X
// исключён» в отчёте «Итога» и сам искал, чей это узел. Теперь причина
// приезжает с ULID, и строка обязана показать её так же, как показывает
// деградации подписки.
//
// Их бывает несколько на один источник (выпали три члена группы) — поэтому
// список, а не одна строка, как у ParseFailedSourceReason.
func EmitWarningsForSource(sourceID string) []string {
	if sourceID == "" {
		return nil
	}
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	var out []string
	for _, e := range buildReport {
		if e.Kind == BuildReportEmitDegraded && e.SourceID == sourceID {
			out = append(out, e.Reason)
		}
	}
	return out
}

// DroppedNodesForSource — сколько узлов источника снял последний рубеж; ноль,
// если источник прошёл сборку целым.
//
// Отдельно от ExcludedSourceReason, потому что это РАЗНЫЕ состояния строки
// Sources: исключённый целиком источник несёт ⚠ с причиной, источник со
// снятыми узлами — мягкую пометку «снято N ⚠» (SPEC 115 §3). Показать вторую
// как первую значило бы объявить потерянным источник, который работает.
func DroppedNodesForSource(sourceID string) (int, string) {
	if sourceID == "" {
		return 0, ""
	}
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	for _, e := range buildReport {
		if e.Kind == BuildReportNodesDropped && e.SourceID == sourceID {
			return e.NodeCount, e.Reason
		}
	}
	return 0, ""
}
