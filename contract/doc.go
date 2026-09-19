// Package contract — каталог общих с LxBox данных и документов. Кода в нём
// два файла и оба без логики: embed.go вшивает реестр в бинарь лаунчера, а
// этот — только держит директиву генерации.
//
// `go generate ./contract/...` пересобирает contract/docs/generated/ из
// реестра. Страницы генерируются, а не пишутся руками: CI-джоба contract
// запускает генератор и падает на `git diff --exit-code contract/docs/generated`,
// если реестр и документация разошлись (SPEC 131 §7).
package contract

//go:generate go run ./tools/gendocs
