package linkmap

// Греп-страж: в пакете движка НЕТ имён схем и протоколов.
//
// Принцип владельца 19.09.2026: реестр несёт примитивы, в коде ОДИН движок,
// общий для всех схем и видов источника. Дефект, от которого страж защищает,
// прокрадывается не крупной веткой, а одной строкой «а вот у этой схемы
// иначе» — и с ней возвращается второй, скрытый диспетчер протокола
// (QUIRKS Q133-21).
//
// Имена берутся из РЕЕСТРА, а не из списка в тесте: список в тесте отстал бы
// от реестра при первой новой схеме.
//
// Проверяется ИСПОЛНЯЕМЫЙ код, а не проза: комментарии обязаны называть вещи
// своими именами («у trojan пароль — весь userinfo»), иначе обоснование
// решения становится нечитаемым. Поэтому перед проверкой снимаются
// комментарии, строковые литералы и путь модуля в import.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config/registry"
)

// extraForbidden — имена, которых в реестре нет схемами, но которые означают
// ровно ту же беду: диалект, названный по имени.
var extraForbidden = []string{
	"vmess", "shadowsocks", "hysteria", "wireguard", "amnezia", "reality",
	"xray", "singbox", "sing-box", "clash",
}

func TestNoSchemeNamesInEngine(t *testing.T) {
	forbidden := map[string]bool{}
	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}
	for _, s := range set.Schemes() {
		if len(s) >= 3 {
			forbidden[strings.ToLower(s)] = true
		}
	}
	reg, err := registry.Get()
	if err != nil {
		t.Fatalf("registry.Get: %v", err)
	}
	for _, s := range reg.Schemes() {
		if len(s) >= 3 {
			forbidden[strings.ToLower(s)] = true
		}
	}
	for _, s := range extraForbidden {
		forbidden[s] = true
	}
	// "chain" и "group" — не протоколы диалекта, а сущности документа; слово
	// "chain" законно встречается в прозе про цепочку вызовов.
	delete(forbidden, "chain")
	delete(forbidden, "group")
	// "http" и "tls" — слова общего словаря (HTTP-заголовок, TLS-блок), по
	// ним страж давал бы ложное срабатывание на каждом комментарии.
	delete(forbidden, "http")
	delete(forbidden, "tls")

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		// Тесты называют схемы законно: они кормят движок живыми ссылками и
		// ждут конкретных тел. Страж защищает ИСПОЛНЯЕМЫЙ код пакета — именно
		// там одна строка «а вот у этой схемы иначе» возвращает второй,
		// скрытый диспетчер протокола.
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0) // 0 = комментарии не читать
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			var text string
			switch v := n.(type) {
			case *ast.Ident:
				text = v.Name
			case *ast.BasicLit:
				if v.Kind != token.STRING {
					return true
				}
				text = v.Value
			case *ast.ImportSpec:
				// Путь модуля несёт имя проекта, а не диалекта.
				return false
			default:
				return true
			}
			low := strings.ToLower(text)
			for name := range forbidden {
				if strings.Contains(low, name) {
					t.Errorf("%s:%d: имя схемы/протокола %q в исполняемом коде движка: %s",
						f, fset.Position(n.Pos()).Line, name, text)
				}
			}
			return true
		})
	}
}
