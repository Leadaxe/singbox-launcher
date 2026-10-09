package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Версии ядра форка и их сравнение. Файл без build-тега: сравнение нужно и
// классификатору локальной службы (daemon_service_state.go, darwin/windows),
// и обслуживанию удалённых машин (SPEC 161), которое идёт с любой платформы,
// включая Linux и Win7.

// coreBuild — версия ядра форка для сравнения: база X.Y.Z, номер релиза
// форка -lx.N и пре-релиз после него (-rc1, -rc.2, -dev).
type coreBuild struct {
	base   [3]int
	lx     int
	pre    bool
	preNum int
}

// parseCoreBuild разбирает "1.14.1-lx.12", "v1.14.1-lx.12-rc1",
// "1.14.1-lx.12-rc.2". Свой разбор, а не CompareVersions: тот сравнивает
// только базу, и lx.10 для него равно lx.11. ok=false — не пронумерованный
// релиз форка: пусто, "unknown", "unnamed-dev", апстрим без -lx.N.
func parseCoreBuild(v string) (coreBuild, bool) {
	var b coreBuild
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	i := strings.Index(v, "-lx.")
	if i < 0 {
		return b, false
	}
	parts := strings.Split(v[:i], ".")
	if len(parts) != len(b.base) {
		return b, false
	}
	for k, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return b, false
		}
		b.base[k] = n
	}
	rest := v[i+len("-lx."):]
	digits := leadingDigits(rest)
	if digits == "" {
		return b, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return b, false
	}
	b.lx = n
	rest = rest[len(digits):]
	if rest == "" {
		return b, true
	}
	if rest[0] != '-' {
		return b, false
	}
	// Пре-релиз: номер — последняя группа цифр (rc1, rc.2); без цифр — 0.
	b.pre = true
	tail := strings.TrimRight(rest, "0123456789")
	if num := rest[len(tail):]; num != "" {
		if n, err := strconv.Atoi(num); err == nil {
			b.preNum = n
		}
	}
	return b, true
}

// leadingDigits — ведущие цифры s.
func leadingDigits(s string) string {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	return s[:end]
}

// compareCoreBuilds: база, затем номер lx, затем релиз выше своего
// пре-релиза (lx.12-rc1 < lx.12), затем номер пре-релиза. -1, 0, 1.
func compareCoreBuilds(a, b coreBuild) int {
	for k := range a.base {
		if c := compareInts(a.base[k], b.base[k]); c != 0 {
			return c
		}
	}
	if c := compareInts(a.lx, b.lx); c != 0 {
		return c
	}
	switch {
	case a.pre && !b.pre:
		return -1
	case !a.pre && b.pre:
		return 1
	case a.pre:
		return compareInts(a.preNum, b.preNum)
	}
	return 0
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// CoreVersionVerdict — итог сравнения версии ядра на машине с требуемой
// лаунчером (constants.RequiredCoreVersion).
type CoreVersionVerdict int

const (
	// CoreVersionUnknown — хотя бы одну сторону не разобрать (dev-сборка,
	// апстрим без -lx.N, пусто). Предупреждения не даёт: ⚠ — только при
	// доказанном Older (SPEC 161 PLAN §2).
	CoreVersionUnknown CoreVersionVerdict = iota
	CoreVersionOlder
	CoreVersionCurrent
	CoreVersionNewer
)

// CompareCoreVersion сравнивает версию работающего ядра с требуемой.
// Пре-релиз ниже своего релиза: lx.14-rc1 < lx.14.
func CompareCoreVersion(running, required string) CoreVersionVerdict {
	a, okA := parseCoreBuild(running)
	b, okB := parseCoreBuild(required)
	if !okA || !okB {
		return CoreVersionUnknown
	}
	switch compareCoreBuilds(a, b) {
	case -1:
		return CoreVersionOlder
	case 1:
		return CoreVersionNewer
	}
	return CoreVersionCurrent
}

// CoreBuildShort — короткое имя сборки форка для строк UI: "1.14.2-lx.11" →
// "lx.11", "1.14.3-lx.14-rc1" → "lx.14-rc1". Не разбирается — версия как есть.
func CoreBuildShort(v string) string {
	v = strings.TrimSpace(v)
	if _, ok := parseCoreBuild(v); !ok {
		return v
	}
	return "lx." + v[strings.Index(v, "-lx.")+len("-lx."):]
}

// CoreVersionPairLabels — подписи двух версий для фразы «running … older than
// required …»: короткие (lx.11 / lx.14), а если короткие совпали при разной
// базе (1.14.2-lx.14 против 1.14.3-lx.14) — полные, чтобы не вышло
// «lx.14 is older than lx.14».
func CoreVersionPairLabels(running, required string) (string, string) {
	a, b := CoreBuildShort(running), CoreBuildShort(required)
	if a == b && strings.TrimSpace(running) != strings.TrimSpace(required) {
		return strings.TrimPrefix(strings.TrimSpace(running), "v"), strings.TrimPrefix(strings.TrimSpace(required), "v")
	}
	return a, b
}

// sha256File — hex sha256 содержимого файла.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
