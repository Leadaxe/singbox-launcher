package backup

// Экспорт состояния лаунчера в переносимый файл.
//
// Писатель один — формат 1.0, сериализация состояния v8 (export10.go). С
// v1.6.0 лаунчер пишет ТОЛЬКО его (решение владельца, D-110): релизы лаунчера
// и LxBox выходят синхронно, и окно «два писателя, дефолт 0.12» (SPEC 127 §4)
// отменено вместе с писателем 0.12 и выбором формата. Импорт по-прежнему
// читает оба формата: файлы 0.x уже у пользователей на руках, и вход для них
// живёт всегда (legacy_read_0x.go).

import (
	"encoding/json"
	"time"

	"singbox-launcher/core/state"
)

// ExportOptions — что подмешать в шапку файла.
type ExportOptions struct {
	// AppVersion — версия лаунчера (exported_by.version).
	AppVersion string
	// Platform — GOOS, для диагностики односторонних полей.
	Platform string
	// Now — момент экспорта; ноль означает time.Now(). Параметр существует
	// ради воспроизводимых тестов, а не ради «настраиваемости».
	Now time.Time
}

// ExportFile пишет бэкап состояния в файл формата 1.0.
//
// Одна точка на UI, debug API и инструменты: вызывающему не нужно знать,
// какая структура получилась, — он получает файл плюс предупреждения
// экспорта.
func ExportFile(path string, s *state.State, opts ExportOptions) ([]Warning, error) {
	b, warns, err := Export10(s, opts)
	if err != nil {
		return warns, err
	}
	return warns, WriteFile10(path, b)
}

// sourceExportName — как назвать источник в предупреждении экспорта.
func sourceExportName(src state.Source) string {
	if src.Name != "" {
		return src.Name
	}
	if n := src.NodeTagOrLabel(); n != "" {
		return n
	}
	return src.ID
}

// exportWarp — WG/MASQUE-регистрации в переносимую форму warp[].
func exportWarp(s *state.State) []json.RawMessage {
	if s.WarpAccounts == nil {
		return nil
	}
	var out []json.RawMessage
	appendAcc := func(typ string, acc any) {
		m := map[string]any{}
		raw, err := json.Marshal(acc)
		if err != nil || json.Unmarshal(raw, &m) != nil {
			return
		}
		m["type"] = typ
		if enc, err := json.Marshal(m); err == nil {
			out = append(out, enc)
		}
	}
	if s.WarpAccounts.WG != nil {
		appendAcc("wg", s.WarpAccounts.WG)
	}
	if s.WarpAccounts.Masque != nil {
		appendAcc("masque", s.WarpAccounts.Masque)
	}
	return out
}

// exportVars отдаёт только переносимые имена переменных.
//
// Реестр (registry/vars.json) помечает portable-имена: те, что означают одно
// и то же на обеих сторонах. Непереносимое (пути, интерфейсы, платформенные
// флаги) на другой машине значит другое, и переносить его — значит молча
// сломать чужую настройку.
func exportVars(vars []state.SettingVar) map[string]string {
	if len(vars) == 0 {
		return nil
	}
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		if !IsPortableVar(v.Name) {
			continue
		}
		out[v.Name] = v.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func routeFinal(s *state.State) string {
	// vars["route_final"] — канонический канал лаунчера (config_params он не
	// заполняет вовсе); config_params — legacy-состояния и чужие фикстуры.
	for _, v := range s.Vars {
		if v.Name == "route_final" && v.Value != "" {
			return v.Value
		}
	}
	for _, p := range s.ConfigParams {
		if p.Name == "final" || p.Name == "route.final" {
			return p.Value
		}
	}
	return ""
}
