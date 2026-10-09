package debugapi

import (
	"net/http"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// mutationLogBodyLimit — сколько байт ответа попадает в строку журнала:
// хватает на diff_summary, applied и error, и строка остаётся одной.
const mutationLogBodyLimit = 400

// logMutations — журнал пишущих вызовов API (SPEC 160 C): каждый POST,
// PATCH, PUT и DELETE оставляет в основном логе лаунчера одну строку —
// метод, путь с query, код ответа и начало тела ответа.
//
// Тело ЗАПРОСА не пишется никогда: ссылки и файлы бэкапа несут секреты.
// GET не журналируется — чтение ничего не меняет, а опрос агента забил бы
// лог. api.log не используется: он занят трафиком Clash API.
func logMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		rec := &mutationRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		debuglog.InfoLog("debugapi: %s %s → %d %s", r.Method, r.URL.RequestURI(), rec.status, rec.preview())
	})
}

// mutationRecorder пропускает ответ насквозь и запоминает код и первые
// mutationLogBodyLimit байт тела.
type mutationRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	head        []byte
}

func (m *mutationRecorder) WriteHeader(code int) {
	if !m.wroteHeader {
		m.status = code
		m.wroteHeader = true
	}
	m.ResponseWriter.WriteHeader(code)
}

func (m *mutationRecorder) Write(b []byte) (int, error) {
	m.wroteHeader = true
	if room := mutationLogBodyLimit - len(m.head); room > 0 {
		if len(b) < room {
			room = len(b)
		}
		m.head = append(m.head, b[:room]...)
	}
	return m.ResponseWriter.Write(b)
}

// preview — начало тела одной строкой: переводы строк и прочие пробельные
// серии схлопнуты в пробел, оборванный на лимите UTF-8-символ отброшен.
func (m *mutationRecorder) preview() string {
	return strings.Join(strings.Fields(strings.ToValidUTF8(string(m.head), "")), " ")
}
