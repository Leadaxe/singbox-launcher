package debugapi

import (
	"net/http"
	"runtime"
	"strconv"
)

// handleGoroutines — GET /debug/goroutines: полный дамп стеков всех горутин
// процесса лаунчера в формате runtime.Stack (тот же текст, что Go печатает
// по SIGQUIT), но без остановки процесса.
//
// Зачем: при зависшем UI Fyne Go-часть продолжает жить (debug API отвечает,
// heartbeat идёт), а увидеть, на чём стоит главная горутина (goroutine 1 =
// цикл GLFW/Fyne) или кто держит fyne.DoAndWait, иначе нельзя — stderr
// лаунчера при запуске из Finder уходит в /dev/null, lldb-attach процесс
// убивает. Отдаём text/plain, чтобы дамп читался и grep-ился как есть.
//
// Буфер растёт удвоением до maxGoroutineDump — runtime.Stack не сообщает
// нужный размер, а обрезанный дамп хуже, чем чуть более долгий сбор.
func (s *Server) handleGoroutines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	buf := dumpAllGoroutines()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Goroutines", strconv.Itoa(runtime.NumGoroutine()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf)
}

const maxGoroutineDump = 64 << 20

func dumpAllGoroutines() []byte {
	for size := 1 << 20; ; size *= 2 {
		buf := make([]byte, size)
		n := runtime.Stack(buf, true)
		if n < size || size >= maxGoroutineDump {
			return buf[:n]
		}
	}
}
