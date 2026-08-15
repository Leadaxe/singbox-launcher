// Package debugapi — SPEC 100 §3.7: произвольные REST- и gRPC-вызовы к
// сопряжённому демону (удалённой машине или локальному lxd).
//
// Passthrough — ТУННЕЛЬ, не прокси: запрос уходит только на управляющий канал
// конкретного сопряжённого демона (его mTLS-ключи и пин из реестра/настроек).
// Никаких запросов на произвольные адреса отсюда сделать нельзя.
//
// gRPC-вызовы резолвятся по имени метода через protoregistry: все дескрипторы
// internal/daemonpb уже зарегистрированы при импорте, protojson конвертирует
// JSON ↔ proto. Ручной таблицы методов нет — новые RPC после обновления
// pb-файлов (SYNC_REV) подхватываются без правок API.
package debugapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	// Регистрация дескрипторов daemon.* в protoregistry.GlobalFiles —
	// фундамент резолва методов по имени.
	_ "singbox-launcher/internal/daemonpb"
)

// grpcPackagePrefix — passthrough говорит только с сервисами демона.
// protoregistry содержит дескрипторы всего бинаря, но канал ведёт к демону, и
// показывать в discovery чужие пакеты значило бы обещать то, чего демон не
// реализует.
const grpcPackagePrefix = "daemon."

// --- REST passthrough -----------------------------------------------------

// rawRESTRequest — тело POST …/raw/rest.
type rawRESTRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Body — JSON-тело запроса; BodyBase64 — бинарное (взаимоисключающие).
	Body        json.RawMessage `json:"body"`
	BodyBase64  string          `json:"body_base64"`
	ContentType string          `json:"content_type"`
}

// rawRESTBodyLimit — свой лимит тела (не decodeJSONBody с его 1 MiB):
// через body_base64 заливают .srs-файлы, а они бывают больше мегабайта.
const rawRESTBodyLimit = 32 << 20

var allowedRawMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodDelete: true, http.MethodPatch: true, http.MethodHead: true,
}

// decodeRawRESTRequest разбирает и валидирует тело raw-REST вызова.
func decodeRawRESTRequest(r *http.Request) (rawRESTRequest, []byte, error) {
	var req rawRESTRequest
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, rawRESTBodyLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, nil, fmt.Errorf("invalid body: %w", err)
	}
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if !allowedRawMethods[req.Method] {
		return req, nil, fmt.Errorf("method must be one of GET/POST/PUT/DELETE/PATCH/HEAD")
	}
	if !strings.HasPrefix(req.Path, "/") {
		return req, nil, fmt.Errorf("path must start with /")
	}
	hasJSON := len(req.Body) > 0 && string(req.Body) != "null"
	hasB64 := strings.TrimSpace(req.BodyBase64) != ""
	if hasJSON && hasB64 {
		return req, nil, fmt.Errorf("body and body_base64 are mutually exclusive")
	}
	var payload []byte
	switch {
	case hasJSON:
		payload = []byte(req.Body)
		if req.ContentType == "" {
			req.ContentType = "application/json"
		}
	case hasB64:
		raw, err := base64.StdEncoding.DecodeString(req.BodyBase64)
		if err != nil {
			return req, nil, fmt.Errorf("body_base64: %w", err)
		}
		payload = raw
		if req.ContentType == "" {
			req.ContentType = "application/octet-stream"
		}
	}
	return req, payload, nil
}

// writeRawRESTResponse отдаёт ответ демона как данные: статус и тело демона
// не интерпретируются (это passthrough), HTTP-статус НАШЕГО ответа — 200.
// Вкладывать чужой статус в свой нельзя: агент не отличил бы «нашу» 404
// (unknown machine) от 404 демона.
func writeRawRESTResponse(w http.ResponseWriter, status int, body []byte, contentType string) {
	out := map[string]any{
		"status":       status,
		"content_type": emptyToNil(contentType),
	}
	if len(body) > 0 {
		if json.Valid(body) {
			out["body"] = json.RawMessage(body)
		} else {
			out["body_base64"] = base64.StdEncoding.EncodeToString(body)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRemoteRawREST — POST /remote/machines/{id}/raw/rest.
func (s *Server) handleRemoteRawREST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	req, payload, err := decodeRawRESTRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	status, body, contentType, err := s.remote.Registry.AdminDo(id, req.Method, req.Path, payload, req.ContentType)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeRawRESTResponse(w, status, body, contentType)
}

// --- gRPC passthrough -----------------------------------------------------

// rawGRPCRequest — тело POST …/raw/grpc.
type rawGRPCRequest struct {
	// Method — полное имя: "/daemon.StartedService/URLTest" (ведущий слэш
	// необязателен, разделитель метода — "/" или ".").
	Method  string          `json:"method"`
	Request json.RawMessage `json:"request"`
	// Timeout — дедлайн unary-вызова (Go duration, default 15s, cap 60s).
	Timeout string `json:"timeout"`
	// Duration / MaxEvents — окно сборки server-stream (default 5s/100).
	Duration  string `json:"duration"`
	MaxEvents int    `json:"max_events"`
}

// resolveGRPCMethod находит дескриптор метода по полному имени.
func resolveGRPCMethod(full string) (protoreflect.MethodDescriptor, error) {
	name := strings.TrimPrefix(strings.TrimSpace(full), "/")
	var svcName, mName string
	if i := strings.LastIndex(name, "/"); i > 0 {
		svcName, mName = name[:i], name[i+1:]
	} else if i := strings.LastIndex(name, "."); i > 0 {
		svcName, mName = name[:i], name[i+1:]
	}
	if svcName == "" || mName == "" {
		return nil, fmt.Errorf("method must look like /daemon.StartedService/GetGroups")
	}
	if !strings.HasPrefix(svcName, grpcPackagePrefix) {
		return nil, fmt.Errorf("only %s* services are reachable through this tunnel", grpcPackagePrefix)
	}
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(svcName))
	if err != nil {
		return nil, fmt.Errorf("unknown service %q", svcName)
	}
	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a service", svcName)
	}
	m := svc.Methods().ByName(protoreflect.Name(mName))
	if m == nil {
		return nil, fmt.Errorf("unknown method %q in %q", mName, svcName)
	}
	return m, nil
}

func grpcMethodKind(m protoreflect.MethodDescriptor) string {
	switch {
	case m.IsStreamingClient() && m.IsStreamingServer():
		return "bidi_stream"
	case m.IsStreamingClient():
		return "client_stream"
	case m.IsStreamingServer():
		return "server_stream"
	default:
		return "unary"
	}
}

// grpcFullMethod — "/pkg.Service/Method" из дескриптора (форма grpc-go).
func grpcFullMethod(m protoreflect.MethodDescriptor) string {
	return fmt.Sprintf("/%s/%s", m.Parent().FullName(), m.Name())
}

// buildGRPCRequestMessage создаёт request-сообщение из JSON (пустой JSON =
// пустое сообщение — так {} и отсутствие request эквивалентны Empty).
func buildGRPCRequestMessage(m protoreflect.MethodDescriptor, reqJSON json.RawMessage) (*dynamicpb.Message, error) {
	in := dynamicpb.NewMessage(m.Input())
	if len(reqJSON) > 0 && string(reqJSON) != "null" {
		if err := protojson.Unmarshal(reqJSON, in); err != nil {
			return nil, fmt.Errorf("request does not match %s: %w", m.Input().FullName(), err)
		}
	}
	return in, nil
}

// protoToJSON — protojson с выводом дефолтных полей: агенту важно видеть
// РОВНО схему ответа, а не гадать, какие поля опущены как zero-value.
var protoToJSON = protojson.MarshalOptions{EmitUnpopulated: true}

// invokeGRPCUnary — один unary-вызов через динамические сообщения.
func invokeGRPCUnary(conn grpc.ClientConnInterface, m protoreflect.MethodDescriptor, reqJSON json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	in, err := buildGRPCRequestMessage(m, reqJSON)
	if err != nil {
		return nil, err
	}
	out := dynamicpb.NewMessage(m.Output())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := conn.Invoke(ctx, grpcFullMethod(m), in, out); err != nil {
		return nil, err
	}
	b, err := protoToJSON.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal response: %w", err)
	}
	return b, nil
}

// invokeGRPCServerStream собирает окно server-stream'а: события до
// maxEvents либо до конца duration; EOF раньше — тоже нормальное завершение.
func invokeGRPCServerStream(conn grpc.ClientConnInterface, m protoreflect.MethodDescriptor, reqJSON json.RawMessage, duration time.Duration, maxEvents int) (events []json.RawMessage, truncated bool, err error) {
	in, err := buildGRPCRequestMessage(m, reqJSON)
	if err != nil {
		return nil, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	sd := &grpc.StreamDesc{StreamName: string(m.Name()), ServerStreams: true}
	stream, err := conn.NewStream(ctx, sd, grpcFullMethod(m))
	if err != nil {
		return nil, false, err
	}
	if err := stream.SendMsg(in); err != nil {
		return nil, false, err
	}
	if err := stream.CloseSend(); err != nil {
		return nil, false, err
	}
	events = []json.RawMessage{}
	for len(events) < maxEvents {
		out := dynamicpb.NewMessage(m.Output())
		recvErr := stream.RecvMsg(out)
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				return events, false, nil // стрим закончился сам
			}
			// Дедлайн окна — штатное завершение сборки, а не ошибка вызова.
			if ctx.Err() != nil {
				return events, false, nil
			}
			return events, false, recvErr
		}
		b, mErr := protoToJSON.Marshal(out)
		if mErr != nil {
			return events, false, fmt.Errorf("marshal event: %w", mErr)
		}
		events = append(events, b)
	}
	return events, true, nil
}

// parseRawGRPCDurations — таймауты запроса с дефолтами и капами.
func parseRawGRPCDurations(req rawGRPCRequest) (timeout, duration time.Duration, maxEvents int, err error) {
	timeout = 15 * time.Second
	if req.Timeout != "" {
		d, perr := time.ParseDuration(req.Timeout)
		if perr != nil || d <= 0 {
			return 0, 0, 0, fmt.Errorf("timeout: invalid duration %q", req.Timeout)
		}
		if d > time.Minute {
			d = time.Minute
		}
		timeout = d
	}
	duration = 5 * time.Second
	if req.Duration != "" {
		d, perr := time.ParseDuration(req.Duration)
		if perr != nil || d <= 0 {
			return 0, 0, 0, fmt.Errorf("duration: invalid duration %q", req.Duration)
		}
		if d > time.Minute {
			d = time.Minute
		}
		duration = d
	}
	maxEvents = 100
	if req.MaxEvents != 0 {
		if req.MaxEvents < 0 {
			return 0, 0, 0, fmt.Errorf("max_events must be positive")
		}
		maxEvents = req.MaxEvents
		if maxEvents > 5000 {
			maxEvents = 5000
		}
	}
	return timeout, duration, maxEvents, nil
}

// serveRawGRPC — общее тело raw-gRPC вызова (remote и daemon различаются
// только источником соединения).
func (s *Server) serveRawGRPC(w http.ResponseWriter, r *http.Request, connOf func() (grpc.ClientConnInterface, error)) {
	var req rawGRPCRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Method) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "method is required, e.g. /daemon.StartedService/GetGroups"})
		return
	}
	m, err := resolveGRPCMethod(req.Method)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	timeout, duration, maxEvents, err := parseRawGRPCDurations(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	// Kind-гейт до dial: на неподдерживаемый вид метода незачем открывать
	// соединение (и жечь таймаут на недоступной машине).
	kind := grpcMethodKind(m)
	if kind != "unary" && kind != "server_stream" {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": fmt.Sprintf("%s is a %s method; client/bidi streams are not supported by the raw tunnel", grpcFullMethod(m), kind),
		})
		return
	}
	conn, err := connOf()
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	switch kind {
	case "unary":
		resp, err := invokeGRPCUnary(conn, m, req.Request, timeout)
		if err != nil {
			writeRemoteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"method":   grpcFullMethod(m),
			"kind":     "unary",
			"response": resp,
		})
	default:
		events, truncated, err := invokeGRPCServerStream(conn, m, req.Request, duration, maxEvents)
		if err != nil {
			writeRemoteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"method":    grpcFullMethod(m),
			"kind":      "server_stream",
			"events":    events,
			"truncated": truncated,
		})
	}
}

// handleRemoteRawGRPC — POST /remote/machines/{id}/raw/grpc.
func (s *Server) handleRemoteRawGRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	s.serveRawGRPC(w, r, func() (grpc.ClientConnInterface, error) {
		t, err := s.remote.Pool.Get(id)
		if err != nil {
			return nil, err
		}
		return t.GRPCConn()
	})
}

// handleGRPCMethods — GET /grpc/methods: discovery для raw-вызовов.
func (s *Server) handleGRPCMethods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	type methodView struct {
		Service string `json:"service"`
		Method  string `json:"method"`
		Full    string `json:"full"`
		Kind    string `json:"kind"`
		Input   string `json:"input"`
		Output  string `json:"output"`
	}
	out := []methodView{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if !strings.HasPrefix(string(fd.Package())+".", grpcPackagePrefix) {
			return true
		}
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			svc := svcs.Get(i)
			methods := svc.Methods()
			for j := 0; j < methods.Len(); j++ {
				m := methods.Get(j)
				out = append(out, methodView{
					Service: string(svc.FullName()),
					Method:  string(m.Name()),
					Full:    grpcFullMethod(m),
					Kind:    grpcMethodKind(m),
					Input:   string(m.Input().FullName()),
					Output:  string(m.Output().FullName()),
				})
			}
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Full < out[j].Full })
	writeJSON(w, http.StatusOK, map[string]any{
		"methods": out,
		"hint":    "POST …/raw/grpc with {method, request}; server streams collect a window ({duration, max_events})",
	})
}
