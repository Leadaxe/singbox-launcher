package debuglog

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type Level uint8

const (
	LevelOff Level = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelVerbose
	LevelTrace

	UseGlobal Level = 255
)

const envKey = "SINGBOX_DEBUG"

var (
	GlobalLevel = parseEnvLevel(os.Getenv(envKey))
)

func parseEnvLevel(raw string) Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "trace":
		return LevelTrace
	case "verbose", "debug":
		return LevelVerbose
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	case "off":
		return LevelOff
	default:
		// По умолчанию показываем DEBUG логи
		return LevelVerbose
	}
}

func Log(prefix string, level Level, local Level, format string, args ...interface{}) {
	effective := GlobalLevel
	if local != UseGlobal {
		effective = local
	}
	if level > effective {
		return
	}
	message := fmt.Sprintf(format, args...)
	if prefix != "" {
		log.Printf("[%s] %s", prefix, message)
	} else {
		log.Print(message)
	}
}

func ShouldLog(level Level, local Level) bool {
	effective := GlobalLevel
	if local != UseGlobal {
		effective = local
	}
	return level <= effective
}

// LogTextFragment логирует фрагмент текста с автоматической обрезкой для читаемости.
// Для больших текстов показывает начало и конец, избегая захламления логов.
//
// Параметры:
//   - prefix: префикс модуля для логов
//   - level: уровень логирования
//   - local: локальный уровень (или UseGlobal)
//   - description: описание фрагмента
//   - text: текст для логирования
//   - maxChars: максимум символов для показа (рекомендуется 500-1000)
func LogTextFragment(prefix string, level Level, local Level, description, text string, maxChars int) {
	if !ShouldLog(level, local) {
		return
	}

	textLen := len(text)

	// Если текст короткий, показываем полностью
	if textLen <= maxChars*2 {
		Log(prefix, level, local, "%s (len=%d): %s", description, textLen, text)
		return
	}

	// Для длинных текстов показываем начало и конец
	Log(prefix, level, local, "%s (len=%d): first %d chars: %s",
		description, textLen, maxChars, text[:maxChars])
	Log(prefix, level, local, "%s (len=%d): last %d chars: %s",
		description, textLen, maxChars, text[textLen-maxChars:])
}
