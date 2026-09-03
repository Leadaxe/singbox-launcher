package state

// Мажор схемы состояния, читаемый БЕЗ загрузки и БЕЗ миграции (SPEC 118 Т10).
//
// Зачем отдельная функция, а не `Load(...).Version`. Load — не наблюдатель, а
// преобразователь: он мигрирует v2–v6 в v7 прямо на диске, а всё, что >= 7,
// разбирает как v7, игнорируя незнакомые ключи минорных добавок. Значит после
// Load ответ на вопрос «какой схемой написан этот файл» уже всегда «седьмой» —
// и гейтить по нему нечего.
//
// Гейт нужен ровно там, где состояние ПЕРЕНОСИТСЯ или ПРАВИТСЯ по частям
// между двумя сторонами, версии которых могут разойтись: копирование профиля с
// машины на машину и частичные PATCH'и правил/DNS удалённой машины. Файл,
// написанный БОЛЕЕ НОВЫМ приложением (мажор 8+), эта сборка прочитает как v7,
// молча выронив всё, чего не знает, и запишет обратно уже без этого. Такой
// «тихий рассинхрон форм» и запрещён: несовпадение мажора — внятный отказ с
// обеими версиями в тексте.
//
// Обратная сторона (файл СТАРШЕ, мажор 2–6) отказом НЕ является: миграция
// v6→v7 для того и написана, и её вход — законная форма.

import (
	"fmt"
	"os"
)

// SchemaMajor — мажор схемы состояния этой сборки.
const SchemaMajor = SchemaVersionV7

// ErrSchemaFileMissing — файла состояния нет. Отдельная ошибка: «нечего
// сверять» и «версии разошлись» — разные ответы вызывающему.
var ErrSchemaFileMissing = ErrNotFound

// SchemaVersionOfFile — мажор схемы, которым написан файл по пути path.
//
// Разбирается ровно шапка: top-level `version` (v2–v4) и `meta.version` (v5+).
// Ни одного из них нет — файл не наш: возвращается 0 и ошибка, чтобы
// вызывающий не принял «не понял» за «совпало».
func SchemaVersionOfFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrSchemaFileMissing
		}
		return 0, fmt.Errorf("state: read %s: %w", path, err)
	}
	return SchemaVersionOfBytes(data)
}

// SchemaVersionOfBytes — то же для уже прочитанных байт.
func SchemaVersionOfBytes(data []byte) (int, error) {
	top, meta, err := sniffSchemaVersion(data)
	if err != nil {
		return 0, fmt.Errorf("state: parse json: %w", err)
	}
	if meta > 0 {
		return meta, nil
	}
	if top > 0 {
		return top, nil
	}
	return 0, fmt.Errorf("state: unknown schema (neither legacy version nor meta.version present)")
}

// SchemaMismatchError — файл написан схемой, которую эта сборка не понимает.
//
// Текст называет ОБЕ версии: пользователь удалённой машины видит не «что-то
// пошло не так», а конкретную причину и направление расхождения — обновлять
// надо ту сторону, что отстала.
type SchemaMismatchError struct {
	// Path — файл, чей мажор не подошёл (для диагностики).
	Path string
	// Found — мажор, которым файл написан.
	Found int
	// Supported — мажор этой сборки.
	Supported int
}

func (e *SchemaMismatchError) Error() string {
	return fmt.Sprintf("state schema mismatch: file %s is written by schema v%d, this build speaks v%d — update the older side before transferring or patching state",
		e.Path, e.Found, e.Supported)
}

// CheckSchemaCompatible — гейт переноса/частичной правки состояния.
//
// Отсутствие файла ошибкой гейта не считается (вернётся ErrSchemaFileMissing —
// вызывающий решает сам: для приёмника copy-from это норма, для PATCH'а —
// 404). Мажор НИЖЕ текущего пропускается: его поднимет миграция при Load.
// Мажор ВЫШЕ — SchemaMismatchError.
func CheckSchemaCompatible(path string) error {
	v, err := SchemaVersionOfFile(path)
	if err != nil {
		return err
	}
	if v > SchemaMajor {
		return &SchemaMismatchError{Path: path, Found: v, Supported: SchemaMajor}
	}
	return nil
}
