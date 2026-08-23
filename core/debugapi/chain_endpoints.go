// File chain_endpoints.go — цепочки: состояние и послойная проба (SPEC 110).
//
// Обе ручки живут в группе демона и регистрируются вместе с ней: данные
// приходят из РАБОТАЮЩЕГО ядра по gRPC, а не из состояния на диске. Цепочка
// в конфиге и цепочка в рантайме — разные вещи: `now` у группы меняется без
// перезаписи конфига, звенья поднимаются и вытесняются по idle_timeout.
//
// Зачем поверх /daemon/raw/grpc, который и так умеет звать GetChains: raw
// отдаёт голый ответ RPC, а диагностика «кто виноват» — это не один вызов, а
// N+1 проб с вычислением дельт. Собирать её на стороне клиента значит писать
// один и тот же цикл в каждом потребителе (UI, curl, агент) и одинаково
// ошибаться в знаке дельты.
package debugapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/config"
	daemonpb "singbox-launcher/internal/daemonpb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Пределы пробы.
//
// probeMaxRepeat — потолок повторов: проба синхронная, и `repeat: 100` по
// цепочке из пяти позиций держал бы HTTP-соединение минутами.
//
// probeDefaultTimeoutMs — на позицию, а не на весь прогон: цель как раз в
// том, чтобы медленный хоп ДОЖДАЛСЯ ответа и показал свою цену, а не выпал
// по общему дедлайну вместе с остальными.
const (
	probeMaxRepeat        = 10
	probeDefaultRepeat    = 2
	probeDefaultTimeoutMs = 15000
	probeMaxTimeoutMs     = 120000
)

// chainEndpoints — таблица группы /chains/*.
func (s *Server) chainEndpoints() []apiEndpoint {
	return []apiEndpoint{
		{"GET", "/chains", true, "Chain runtime state (positions, resolved node, clones)", s.handleChains},
		{"POST", "/chains/{tag}/probe", true, "Layer-by-layer latency probe of one chain", s.handleChainProbe},
	}
}

// startedClient — клиент к gRPC-плоскости локального демона.
//
// Соединением владеет вызывающий (тот же контракт, что у raw-хендлера):
// loopback-dial дёшев, пул тут не оправдан.
func (s *Server) startedClient() (daemonpb.StartedServiceClient, func(), error) {
	conn, err := s.daemon.GRPCConn()
	if err != nil {
		return nil, nil, err
	}
	return daemonpb.NewStartedServiceClient(conn), func() { _ = conn.Close() }, nil
}

// handleChains — GET /chains.
func (s *Server) handleChains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()

	list, err := client.GetChains(r.Context(), &emptypb.Empty{})
	if err != nil {
		writeJSON(w, chainRPCStatus(err), map[string]any{"error": chainRPCMessage(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chains": chainViews(list.GetChains())})
}

// chainView / chainPositionView — JSON-форма ответа.
//
// Своя, не protojson: у сгенерированных структур `omitempty` съедает нули, а
// здесь ноль значащий — `errors: 0` и `activeConns: 0` это утверждение
// «ошибок нет», «соединений нет», а не «поле не пришло». Для диагностики
// разница между «ноль» и «неизвестно» принципиальна.
type chainView struct {
	Tag           string              `json:"tag"`
	Positions     []chainPositionView `json:"positions"`
	Dials         int64               `json:"dials"`
	Errors        int64               `json:"errors"`
	ClonesCreated int64               `json:"clones_created"`
	ClonesEvicted int64               `json:"clones_evicted"`
	LiveClones    int64               `json:"live_clones"`
}

type chainPositionView struct {
	Pos int    `json:"pos"`
	Tag string `json:"tag"`
	// ProbeTag — тег для URLTestOutbound: путь ОТ КЛИЕНТА до этой позиции
	// включительно. Отдаётся явно, чтобы клиент не собирал `T#i` сам:
	// теги цепочек содержат эмодзи и решётки, и склейка на стороне
	// потребителя — лишнее место для ошибки.
	ProbeTag    string          `json:"probe_tag"`
	IsGroup     bool            `json:"is_group"`
	Now         string          `json:"now,omitempty"`
	Transparent bool            `json:"transparent"`
	Errors      int64           `json:"errors"`
	Clone       *chainCloneView `json:"clone,omitempty"`
}

type chainCloneView struct {
	State           string   `json:"state,omitempty"`
	ActiveConns     int64    `json:"active_conns"`
	LastPickedAgeMs int64    `json:"last_picked_age_ms"`
	CreatedAgeMs    int64    `json:"created_age_ms"`
	MTUConfigured   uint32   `json:"mtu_configured,omitempty"`
	MTUEffective    uint32   `json:"mtu_effective,omitempty"`
	MTUReason       string   `json:"mtu_reason,omitempty"`
	Stripped        []string `json:"stripped,omitempty"`
	Rewritten       bool     `json:"rewritten"`
	LastError       string   `json:"last_error,omitempty"`
}

func chainViews(chains []*daemonpb.ChainState) []chainView {
	out := make([]chainView, 0, len(chains))
	for _, c := range chains {
		if c == nil {
			continue
		}
		positions := make([]chainPositionView, 0, len(c.GetPositions()))
		for i, p := range c.GetPositions() {
			if p == nil {
				continue
			}
			pv := chainPositionView{
				Pos:         i,
				Tag:         p.GetTag(),
				ProbeTag:    layerTag(c.GetTag(), i),
				IsGroup:     p.GetIsGroup(),
				Now:         p.GetNow(),
				Transparent: p.GetTransparent(),
				Errors:      p.GetErrors(),
			}
			if cl := p.GetClone(); cl != nil {
				pv.Clone = &chainCloneView{
					State:           cl.GetState(),
					ActiveConns:     cl.GetActiveConns(),
					LastPickedAgeMs: cl.GetLastPickedAgeMs(),
					CreatedAgeMs:    cl.GetCreatedAgeMs(),
					MTUConfigured:   cl.GetMtuConfigured(),
					MTUEffective:    cl.GetMtuEffective(),
					MTUReason:       cl.GetMtuReason(),
					Stripped:        cl.GetStripped(),
					Rewritten:       cl.GetRewritten(),
					LastError:       cl.GetLastError(),
				}
			}
			positions = append(positions, pv)
		}
		out = append(out, chainView{
			Tag:           c.GetTag(),
			Positions:     positions,
			Dials:         c.GetDials(),
			Errors:        c.GetErrors(),
			ClonesCreated: c.GetClonesCreated(),
			ClonesEvicted: c.GetClonesEvicted(),
			LiveClones:    c.GetLiveClones(),
		})
	}
	return out
}

// layerTag — служебный тег префикса цепочки: `T#i` меряет путь от клиента до
// позиции i включительно. Регистрируется ядром, но намеренно отсутствует в
// GetOutbounds и Clash API — увидеть его можно только зная схему имени.
//
// Схема имени принадлежит ядру и собирается общим config.ChainLayerTag —
// локальная копия формата разъехалась бы с распознавателем ChainInternalTag.
func layerTag(chainTag string, pos int) string {
	return config.ChainLayerTag(chainTag, pos)
}

// handleChainProbe — POST /chains/{tag}/probe.
//
// Меряет ПРЕФИКСЫ: `T#0`, `T#1`, … `T#(N-1)`, затем саму цепочку `T`. Цена
// хопа — разность соседних замеров, и получить её отдельной пробой нельзя:
// хоп i недостижим иначе как через i-1, поэтому «сколько стоит именно он» —
// всегда вычитание, а не измерение.
func (s *Server) handleChainProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	tag := pathParam(r, "tag")
	if strings.TrimSpace(tag) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chain tag is required in the path"})
		return
	}

	var req struct {
		Repeat    int    `json:"repeat"`
		TimeoutMs uint32 `json:"timeout_ms"`
		Link      string `json:"link"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	// repeat по умолчанию 2, а не 1: первый прогон поднимает туннели
	// (WG-хендшейк, QUIC-сессия) и завышен в разы. Один замер по умолчанию
	// систематически обвинял бы не тот хоп.
	repeat := req.Repeat
	if repeat <= 0 {
		repeat = probeDefaultRepeat
	}
	if repeat > probeMaxRepeat {
		repeat = probeMaxRepeat
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = probeDefaultTimeoutMs
	}
	if timeoutMs > probeMaxTimeoutMs {
		timeoutMs = probeMaxTimeoutMs
	}
	// Пустой link — тот же эндпоинт, что у UI-пробы: иначе debug-цифры
	// мерялись бы по дефолту ядра, и «почему разошлось с окном Info»
	// становилось бы ложной загадкой.
	if strings.TrimSpace(req.Link) == "" {
		req.Link = api.GetPingTestURL()
	}

	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()

	// Позиции берём из ядра, а не из состояния на диске: пробовать имеет
	// смысл ровно ту цепочку, которая сейчас в рантайме. Конфиг мог уйти
	// вперёд после правки без перезапуска, и проба по нему мерила бы
	// позиции, которых в работающем ядре нет.
	list, err := client.GetChains(r.Context(), &emptypb.Empty{})
	if err != nil {
		writeJSON(w, chainRPCStatus(err), map[string]any{"error": chainRPCMessage(err)})
		return
	}
	var target *daemonpb.ChainState
	for _, c := range list.GetChains() {
		if c.GetTag() == tag {
			target = c
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":  "no chain with tag " + tag + " in the running core",
			"chains": chainTags(list.GetChains()),
		})
		return
	}
	positions := target.GetPositions()
	if len(positions) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": "chain " + tag + " has no positions",
		})
		return
	}

	runs := make([]probeRun, 0, repeat)
	for n := 0; n < repeat; n++ {
		run := probeRun{
			// Прогрев помечается, а не отбрасывается: пользователю важно
			// видеть, что первый прогон был холодным, — иначе «почему
			// цифры разъехались» становится новым вопросом.
			WarmUp: repeat > 1 && n == 0,
			Layers: make([]probeLayer, 0, len(positions)),
		}
		for i, p := range positions {
			delay, errText, rpcErr := probeOnce(r.Context(), client, layerTag(tag, i), req.Link, timeoutMs)
			if rpcErr != nil {
				writeJSON(w, chainRPCStatus(rpcErr), map[string]any{"error": chainRPCMessage(rpcErr)})
				return
			}
			run.Layers = append(run.Layers, probeLayer{
				Pos:         i,
				Tag:         p.GetTag(),
				ProbeTag:    layerTag(tag, i),
				Transparent: p.GetTransparent(),
				DelayMs:     delay,
				Error:       errText,
			})
		}
		// Сама цепочка сверх позиций: её замер включает то, чего нет ни в
		// одном префиксе, — выбор звена и обвязку самого outbound'а типа
		// chain. Расхождение с последним префиксом само по себе диагноз.
		total, totalErr, rpcErr := probeOnce(r.Context(), client, tag, req.Link, timeoutMs)
		if rpcErr != nil {
			writeJSON(w, chainRPCStatus(rpcErr), map[string]any{"error": chainRPCMessage(rpcErr)})
			return
		}
		run.TotalMs = total
		run.TotalError = totalErr
		runs = append(runs, runDeltas(run))
		if r.Context().Err() != nil {
			break
		}
	}
	if len(runs) == 0 {
		writeJSON(w, http.StatusRequestTimeout, map[string]any{"error": "probe cancelled before any run completed"})
		return
	}

	// Дельты и виновник — по ПОСЛЕДНЕМУ прогону: он прогрет, тогда как
	// первый мерил подъём туннелей.
	last := runs[len(runs)-1]
	resp := map[string]any{
		"tag":        tag,
		"runs":       runs,
		"deltas":     last.Deltas,
		"repeat":     repeat,
		"link":       req.Link,
		"timeout_ms": timeoutMs,
	}
	if w := worstLayer(last); w != nil {
		resp["worst"] = w
	}
	writeJSON(w, http.StatusOK, resp)
}

// probeRun — один полный прогон по всем позициям.
type probeRun struct {
	WarmUp bool         `json:"warm_up"`
	Layers []probeLayer `json:"layers"`
	// TotalMs — замер самой цепочки (тег без `#i`).
	TotalMs    uint32       `json:"total_ms"`
	TotalError string       `json:"total_error,omitempty"`
	Deltas     []probeDelta `json:"deltas"`
}

type probeLayer struct {
	Pos      int    `json:"pos"`
	Tag      string `json:"tag"`
	ProbeTag string `json:"probe_tag"`
	// Transparent — позиция схлопнута (в ней выбран direct). Её дельта
	// заведомо около нуля, и «виновником» такая позиция быть не может.
	Transparent bool   `json:"transparent"`
	DelayMs     uint32 `json:"delay_ms"`
	// Error — текст от ядра. НЕ проглатывается: «медленно» и «не
	// поднялось» — разные диагнозы, и второй ядро формулирует само,
	// с указанием позиции и пути до неё.
	Error string `json:"error,omitempty"`
}

// probeDelta — цена одного хопа.
type probeDelta struct {
	Pos int    `json:"pos"`
	Tag string `json:"tag"`
	// CostMs — сырая разность, может быть отрицательной: пробы идут в
	// разное время по разным маршрутам, и шум порядка десятков мс
	// нормален. Отдаётся как есть, чтобы клиент не гадал, что было
	// обрезано.
	CostMs int64 `json:"cost_ms"`
	// CostClamped — та же цена, но отрицательная сведена к нулю: именно
	// её показывает UI, потому что «этот хоп ускорил маршрут» —
	// бессмысленное для пользователя утверждение.
	CostClamped int64 `json:"cost_clamped_ms"`
	// Unmeasured — предыдущий или текущий замер не удался, разность
	// смысла не имеет. Без этого флага дельта от нулевого delay выглядела
	// бы как «хоп бесплатный», хотя он попросту не ответил.
	Unmeasured bool `json:"unmeasured,omitempty"`
}

// runDeltas считает цену каждого хопа как разность соседних префиксов.
func runDeltas(run probeRun) probeRun {
	deltas := make([]probeDelta, 0, len(run.Layers))
	for i, l := range run.Layers {
		d := probeDelta{Pos: l.Pos, Tag: l.Tag}
		switch {
		case l.Error != "":
			d.Unmeasured = true
		case i == 0:
			d.CostMs = int64(l.DelayMs)
		case run.Layers[i-1].Error != "":
			// Опорной точки нет: цену этого хопа не вычислить, хотя сам
			// он ответил.
			d.Unmeasured = true
		default:
			d.CostMs = int64(l.DelayMs) - int64(run.Layers[i-1].DelayMs)
		}
		if d.CostMs > 0 {
			d.CostClamped = d.CostMs
		}
		deltas = append(deltas, d)
	}
	run.Deltas = deltas
	return run
}

// worstLayer — позиция с максимальной положительной дельтой.
//
// Схлопнутые и неизмеренные позиции не рассматриваются: у первых цены нет по
// построению, у вторых она неизвестна — назвать их виновником значило бы
// отправить пользователя менять исправный хоп.
func worstLayer(run probeRun) *probeDelta {
	var best *probeDelta
	for i := range run.Deltas {
		d := run.Deltas[i]
		if d.Unmeasured || d.CostClamped <= 0 {
			continue
		}
		if i < len(run.Layers) && run.Layers[i].Transparent {
			continue
		}
		if best == nil || d.CostClamped > best.CostClamped {
			cp := d
			best = &cp
		}
	}
	return best
}

// probeOnce — одна проба одного тега.
//
// Ошибка ЯДРА приходит полем Error в успешном ответе, а не gRPC-ошибкой:
// первое — «хоп не поднялся» (диагноз, который и нужен пользователю),
// второе — «не смогли поговорить с демоном» (обрыв всей операции). Их
// возвраты разделены, чтобы вызывающий не путал одно с другим.
func probeOnce(
	ctx context.Context,
	client daemonpb.StartedServiceClient,
	tag, link string,
	timeoutMs uint32,
) (uint32, string, error) {
	// Запас поверх таймаута ядра: ядро режет саму пробу, а этот дедлайн
	// страхует от повисшего вызова к демону.
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond+5*time.Second)
	defer cancel()

	resp, err := client.URLTestOutbound(callCtx, &daemonpb.URLTestOutboundRequest{
		OutboundTag: tag,
		Link:        link,
		Timeout:     timeoutMs,
	})
	if err != nil {
		return 0, "", err
	}
	return resp.GetDelay(), resp.GetError(), nil
}

func chainTags(chains []*daemonpb.ChainState) []string {
	out := make([]string, 0, len(chains))
	for _, c := range chains {
		out = append(out, c.GetTag())
	}
	return out
}

// chainRPCStatus — HTTP-код для ошибки gRPC.
//
// Unimplemented выделен: ядро собрано без `with_lx_command`, метод объявлен
// в proto, но реализации нет. Это не сбой связи и не лечится повтором —
// 501 говорит клиенту «пересобери/обнови ядро», а не «попробуй ещё раз».
// FailedPrecondition — ядро остановлено: цепочек в рантайме просто нет.
func chainRPCStatus(err error) int {
	switch status.Code(err) {
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Unavailable:
		return http.StatusBadGateway
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.FailedPrecondition, codes.InvalidArgument:
		return http.StatusUnprocessableEntity
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

// chainRPCMessage — текст ошибки с подсказкой там, где она не самоочевидна.
func chainRPCMessage(err error) string {
	if status.Code(err) == codes.Unimplemented {
		return "core lacks chain diagnostics: rebuild sing-box with -tags with_lx_command (" + err.Error() + ")"
	}
	return err.Error()
}
