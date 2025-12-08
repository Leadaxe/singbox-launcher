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
