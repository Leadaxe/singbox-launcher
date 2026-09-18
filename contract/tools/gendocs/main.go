// Command gendocs собирает человекочитаемую документацию контракта из
// реестра (contract/registry/*.json) в contract/docs/generated/.
//
// Единственный источник правды — реестр: генератор не знает ни одной схемы по
// имени, ни одного кода и ни одного правила; всё приходит с диска. Схемы тела
// читаются загрузчиком лаунчера (core/config/registry) — второго парсера
// секции body в репозитории нет. Секции `uri.*` и метаданные схемы
// (scheme/kind/singbox_type/sources/extension) загрузчику не нужны и им не
// разбираются, поэтому здесь они читаются из того же встроенного файла
// напрямую, без интерпретации значений.
//
// Атрибут `impl` — заметка для разработчика — в документацию не попадает
// (contract/schema/registry_body.schema.json).
//
// Вывод детерминирован: всюду, где порядок не нормирован реестром (карты
// fields, варианты, коды warnings), ключи сортируются; повторный запуск не
// меняет ни байта, и CI сверяет это через `git diff --exit-code`.
//
// go1.20-совместимо (Win7-джоба собирает весь модуль тулчейном go1.20):
// без slices/maps/min/max/clear.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"singbox-launcher/core/config/registry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	outDir := filepath.Join(root, "contract", "docs", "generated")

	reg, err := registry.Load()
	if err != nil {
		return err
	}
	raw, err := loadRaw(reg)
	if err != nil {
		return err
	}
	// VERSION лежит рядом с реестром, но в бинарь не вшит (его читают люди и
	// раннеры обеих сторон), поэтому берётся с диска от корня модуля.
	vdata, err := os.ReadFile(filepath.Join(root, "contract", "VERSION"))
	if err != nil {
		return err
	}
	raw.version = strings.TrimSpace(string(vdata))

	pages := map[string]string{}

	// Общие суб-схемы — отдельные страницы, на них ссылаются схемы протоколов.
	for _, sub := range subSchemaPages(raw) {
		pages["protocols/"+sub.file] = sub.render()
	}

	schemes := raw.schemes()
	for _, scheme := range schemes {
		body, _ := reg.Body(scheme)
		pages["protocols/"+scheme+".md"] = renderScheme(raw, scheme, body)
	}

	pages["warnings.md"] = renderWarnings(raw, reg)
	pages["index.md"] = renderIndex(raw, reg, schemes)

	names := make([]string, 0, len(pages))
	for name := range pages {
		names = append(names, name)
	}
	sort.Strings(names)

	written := map[string]bool{}
	for _, name := range names {
		path := filepath.Join(outDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(pages[name]), 0o644); err != nil {
			return err
		}
		written[filepath.ToSlash(name)] = true
	}

	// Страница, которой в реестре больше нет, не должна пережить схему:
	// иначе CI зелёный, а документация врёт.
	if err := pruneStale(outDir, written); err != nil {
		return err
	}
	fmt.Printf("gendocs: %d страниц → %s\n", len(names), outDir)
	return nil
}

// moduleRoot — корень модуля. Генератор зовётся и из корня репозитория, и из
// каталога contract (go generate ./contract/...), поэтому корень ищется по
// go.mod вверх от рабочего каталога.
func moduleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("не найден корень модуля (go.mod) вверх от %s", wd)
		}
		dir = parent
	}
}

func pruneStale(outDir string, written map[string]bool) error {
	return filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, rerr := filepath.Rel(outDir, path)
		if rerr != nil {
			return rerr
		}
		if written[filepath.ToSlash(rel)] {
			return nil
		}
		return os.Remove(path)
	})
}
