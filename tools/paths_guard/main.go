// paths_guard — страж «в AppDir не пишем» (SPEC 135 §8).
//
// AST-скан по всем пакетам проекта ловит два обхода защиты именованных
// типов internal/paths (AppDir — только чтение, DataDir/LogDir — запись,
// компилятор не даёт передать одно вместо другого напрямую):
//
//	paths-conversion    вызов paths.DataDir(X)/paths.LogDir(X), где исходный
//	                    текст X содержит признак AppDir (в т.ч. через
//	                    string(...))
//	paths-direct-write  os./ioutil.-запись (WriteFile, Create, Rename, …),
//	                    у которой исходный текст аргумента-пути содержит
//	                    признак AppDir
//
// Allowlist по пути файла — в scan.go, с обоснованием на каждую строку.
//
// Запуск из корня: go run ./tools/paths_guard [--strict]
package main

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var skipDirs = map[string]bool{".git": true, "dist": true, "temp": true}

func main() {
	strict := len(os.Args) > 1 && os.Args[1] == "--strict"

	fset := token.NewFileSet()
	var findings []Finding
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && path != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fs, err := ScanSource(fset, path, src)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		findings = append(findings, fs...)
		return nil
	})
	if err != nil {
		fatal("scan: %v", err)
	}

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Pos.String() < findings[j].Pos.String()
	})

	for _, f := range findings {
		fmt.Printf("%s:%d:%d: %s\n", filepath.ToSlash(f.Pos.Filename), f.Pos.Line, f.Pos.Column, f.Rule)
	}
	fmt.Printf("paths_guard: %d findings\n", len(findings))

	if strict && len(findings) > 0 {
		os.Exit(1)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "paths_guard: "+format+"\n", args...)
	os.Exit(2)
}
