// hardcoded_check — ratchet на хардкод-литералы в display-позициях
// (SPEC 111, этап 5).
//
// Строковый литерал с буквами в display-позиции Fyne/диалога
// (display_positions.json), не обёрнутый в locale.* и не помеченный
// `// l10n-exempt: <причина>`, — находка. Baseline
// (hardcoded_baseline.json, {файл: число сайтов}) пуст: любой новый
// хардкод роняет CI. Правило ratchet'а: счётчик файла не растёт и файл,
// которого нет в baseline, не появляется. --write-baseline перезаписывает
// baseline текущими счётчиками (точечный hotfix).
//
// Аргументы функций из l10n_helpers.json сюда не входят — по контракту
// это ключи, их валидирует l10n_check.
//
// Запуск из корня: go run ./tools/l10n/hardcoded_check [--strict] [--write-baseline]
package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	positionsPath = "tools/l10n/display_positions.json"
	baselinePath  = "tools/l10n/hardcoded_baseline.json"
)

var skipDirs = map[string]bool{"tools": true, "dist": true, "temp": true, ".git": true}

// Пакеты, где английский легитимен весь (SPEC 111 §8): логи, debug API.
var skipPkgPrefixes = []string{"internal/debuglog", "core/debugapi"}

func main() {
	strict, write := false, false
	for _, a := range os.Args[1:] {
		switch a {
		case "--strict":
			strict = true
		case "--write-baseline":
			write = true
		default:
			fatal("unknown flag %q", a)
		}
	}

	var positions Positions
	data, err := os.ReadFile(positionsPath)
	if err != nil {
		fatal("read %s: %v", positionsPath, err)
	}
	if err := json.Unmarshal(data, &positions); err != nil {
		fatal("parse %s: %v", positionsPath, err)
	}

	baseline := map[string]int{}
	if data, err := os.ReadFile(baselinePath); err == nil {
		if err := json.Unmarshal(data, &baseline); err != nil {
			fatal("parse %s: %v", baselinePath, err)
		}
	}

	fset := token.NewFileSet()
	counts := map[string]int{}
	var all []Site
	err = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && path != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, pfx := range skipPkgPrefixes {
			if strings.HasPrefix(path, pfx) {
				return nil
			}
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sites, err := ScanFile(fset, path, src, positions)
		if err != nil {
			return err
		}
		counts[path] += len(sites)
		all = append(all, sites...)
		return nil
	})
	if err != nil {
		fatal("scan: %v", err)
	}

	if write {
		out := map[string]int{}
		for f, n := range counts {
			if n > 0 {
				out[f] = n
			}
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		if err := os.WriteFile(baselinePath, append(data, '\n'), 0644); err != nil {
			fatal("write baseline: %v", err)
		}
		fmt.Printf("[hardcoded_check] baseline written: %d files, %d sites\n", len(out), len(all))
		return
	}

	fails := 0
	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	for _, s := range all {
		allowed := baseline[s.File]
		if counts[s.File] > allowed {
			fmt.Printf("FAIL hardcoded %s:%d %q\n", s.File, s.Line, s.Text)
			fails++
		} else {
			fmt.Printf("warn grandfathered %s:%d %q\n", s.File, s.Line, s.Text)
		}
	}
	fmt.Printf("[hardcoded_check] sites: %d, files over baseline: %d\n", len(all), fails)
	if fails > 0 || (strict && len(all) > countGrandfathered(baseline)) {
		os.Exit(1)
	}
}

func countGrandfathered(baseline map[string]int) int {
	n := 0
	for _, c := range baseline {
		n += c
	}
	return n
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "hardcoded_check: "+format+"\n", args...)
	os.Exit(2)
}
