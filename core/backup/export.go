package backup

// Экспорт состояния лаунчера в переносимый файл: точка выбора формата и то,
// что у обоих писателей общее.
//
// Писателей два (SPEC 127 §4, окно совместимости): 1.0 — сериализация
// состояния v8 (export10.go), 0.12 — прежний маппер (legacy_write_012.go).
// Дефолт держит одна константа BackupExportFormatDefault: пока релиз LxBox с
// чтением 1.0 не доехал до пользователей, лаунчер по умолчанию пишет 0.12, а
// пользователь может выбрать 1.0 чекбоксом в диалоге экспорта. Импорт читает
// оба формата всегда.

import (
	"encoding/json"
	"strings"
	"time"

	"singbox-launcher/core/state"
)

// ExportFormat — какой из двух писателей ведёт файл.
type ExportFormat int

const (
	// ExportFormat012 — переходный формат 0.12 (legacy_write_012.go).
	// Числовой ноль намеренно: вызывающий, который про формат ещё не знает,
	// получает прежнее поведение, а не пустой файл.
	ExportFormat012 ExportFormat = iota
	// ExportFormat10 — контракт 1.0: файл = состояние v8 (export10.go).
	ExportFormat10
)

// BackupExportFormatDefault — что пишется, когда формат не выбран явно.
//
// ОДНА константа на всё приложение (SPEC 127 §4): переключение дефолта после
// выхода релиза LxBox с чтением 1.0 — правка этой строки, а не обход
// вызывающих. Пока стоит 0.12: файл, который телефон не прочитает, хуже
// файла старого формата.
const BackupExportFormatDefault = ExportFormat012

// ExportOptions — что подмешать в шапку файла и каким писателем писать.
type ExportOptions struct {
	// AppVersion — версия лаунчера (exported_by.version).
	AppVersion string
	// Platform — GOOS, для диагностики односторонних полей.
	Platform string
	// Now — момент экспорта; ноль означает time.Now(). Параметр существует
	// ради воспроизводимых тестов, а не ради «настраиваемости».
	Now time.Time
	// Format — писатель. Нулевое значение = ExportFormat012, то есть
	// умолчание совпадает с BackupExportFormatDefault без дополнительной
	// проверки у каждого вызывающего.
	Format ExportFormat
}

// ExportFile пишет бэкап состояния в файл выбранным форматом.
//
// Одна точка на оба писателя: вызывающему (UI, debug API, инструменты) не
// нужно знать, какая структура получилась, — он выбирает формат и получает
// файл плюс предупреждения экспорта.
func ExportFile(path string, s *state.State, opts ExportOptions) ([]Warning, error) {
	switch opts.Format {
	case ExportFormat10:
		b, warns, err := Export10(s, opts)
		if err != nil {
			return warns, err
		}
		return warns, WriteFile10(path, b)
	default:
		b, warns, err := Export012(s, opts)
		if err != nil {
			return warns, err
		}
		return warns, WriteFile(path, b)
	}
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

// droppedLocalOnlyFields — перечень per-source настроек подписки, у которых
// в схеме дома нет. Пусто = терять нечего.
//
// С контрактом 0.12 здесь остался ОДИН ключ: UA и HWID-семейство уехали в
// объект identity, а relays_in_directions — нет. Он про то, предлагать ли
// служебные узлы (релеи BYPASS) в списке целей Направлений; у LxBox такой
// развилки нет вовсе, и односторонний ключ в общей схеме был бы ровно тем
// тайным грузом, ради сноса которого убран механизм extensions.
//
// Потеря не косметическая: на маршрут галка не влияет (релей материализуется
// всегда), но после restore список целей будет другим, и пользователь не
// поймёт, куда делся выбор, — поэтому её называют поимённо.
func droppedLocalOnlyFields(src state.Source) string {
	var fields []string
	if src.RelaysInDirections {
		fields = append(fields, "relays_in_directions")
	}
	return strings.Join(fields, ", ")
}

// dedupRefs — наборы srs-правила без повторов, порядок сохранён (тот же канон,
// что у NewSrsRule и у вида).
func dedupRefs(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
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
