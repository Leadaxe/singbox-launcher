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

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// ExportOptions — что подмешать в шапку файла, и то, что писатель берёт у
// шаблона, а не у состояния.
type ExportOptions struct {
	// AppVersion — версия лаунчера (exported_by.version).
	AppVersion string
	// Platform — GOOS, для диагностики односторонних полей.
	Platform string
	// Now — момент экспорта; ноль означает time.Now(). Параметр существует
	// ради воспроизводимых тестов, а не ради «настраиваемости».
	Now time.Time

	// Directions — Направления состояния ПОСЛЕ слияния: тело шаблона или
	// пресета, патчи пресетов и USER-патч поверх (build.ResolveDirections).
	// Писатель берёт тело записи отсюда по тегу. Ссылочная запись в самом
	// состоянии — только tag+ref+updates, и без этого вида в файл уехал бы
	// один тег (BACKUP.md §10, «Цена канонизации Направлений»).
	//
	// Слияние считает вызывающий: ему доступен шаблон, а core/backup о
	// шаблоне не знает. nil — записи состояния как есть; это верно только
	// для прямых записей (ref == "").
	Directions []configtypes.Direction

	// BlockTag — тег блокировки шаблона (TemplateData.DirectionBlockTag), по
	// нему ставится `include_block`: тот же тег, что у галки формы
	// Направления. Пусто — `block-out`.
	BlockTag string

	// RecordVars — объявления шаблона для значений переменных записи
	// (SPEC 129, template.RecordVarDeclsFor). Писатель приводит КОПИЮ
	// состояния к нормам: корневые `dns_<tag>_<var>` — в записи серверов (Н8),
	// без умолчаний и необъявленных имён (Н2–Н4). Без этого состояние, не
	// сохранённое после обновления (debug API экспортирует прямо с диска),
	// уехало бы в файл без маршрута DNS: склеенные имена непереносимы. nil —
	// шаблона нет, записи едут как есть.
	RecordVars *state.RecordVarDecls
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
