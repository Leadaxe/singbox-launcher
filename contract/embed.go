// Package contract встраивает реестр контракта в бинарь лаунчера.
//
// Каталог contract — данные и документы, общие с LxBox; этот файл —
// единственный код в нём и никакой логики не содержит: он только отдаёт
// файлы реестра тем пакетам лаунчера, которые по ним работают
// (core/config/registry → core/config/nodeflow, SPEC 131). LxBox читает те
// же JSON'ы своими средствами и этот файл игнорирует.
package contract

import (
	"embed"
	"io/fs"
)

// Registry — файлы реестра контракта, вшитые в бинарь.
//
//go:embed registry/*.json registry/protocols/*.json
var Registry embed.FS

// ReadRegistry читает файл реестра по имени относительно каталога registry
// ("tls.json", "protocols/vless.json").
func ReadRegistry(name string) ([]byte, error) {
	return fs.ReadFile(Registry, "registry/"+name)
}
