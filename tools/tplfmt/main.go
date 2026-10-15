// tplfmt — форматирует JSON-файл шаблона в компактном каноне internal/jsonfmt:
// короткие объекты и массивы в одну строку (до 100 символов), длинные — по
// элементу на строку, порядок ключей и литералы чисел как в исходнике.
//
// Запуск из корня репозитория:
//
//	go run ./tools/tplfmt <файл>          печатает результат в stdout
//	go run ./tools/tplfmt -w <файл>       форматирует файл на месте
//	go run ./tools/tplfmt -check <файл>   код 1, если файл отформатирован не так
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"singbox-launcher/internal/jsonfmt"
)

func main() {
	write := flag.Bool("w", false, "write the result back to the file")
	check := flag.Bool("check", false, "exit with code 1 if the file is not formatted")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: tplfmt [-w | -check] <file>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 || (*write && *check) {
		flag.Usage()
		os.Exit(2)
	}
	path := flag.Arg(0)

	raw, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	out, err := jsonfmt.FormatCompact(raw)
	if err != nil {
		fail(fmt.Errorf("%s: %w", path, err))
	}

	switch {
	case *check:
		if !bytes.Equal(raw, out) {
			fmt.Fprintf(os.Stderr, "tplfmt: %s is not formatted; run: go run ./tools/tplfmt -w %s\n", path, path)
			os.Exit(1)
		}
	case *write:
		if bytes.Equal(raw, out) {
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
			fail(err)
		}
	default:
		if _, err := os.Stdout.Write(out); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tplfmt:", err)
	os.Exit(1)
}
