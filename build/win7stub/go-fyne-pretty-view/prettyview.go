// Package prettyview — пустая заглушка go-fyne-pretty-view для Win7-сборки.
//
// Настоящий модуль требует fyne v2.8 и go1.26 и подключается только в
// internal/fynewidget/json_editor_pretty.go за ограничением `go1.26`; на
// Go 1.20 этот файл не собирается. Но шаг CI `go get -modfile=go.win7.mod
// ./...` резолвит импорты всех файлов независимо от build-тегов и без
// заглушки поднимал бы в go.win7.mod fyne до v2.8 и golang.org/x/net до
// версии с пакетами iter/cmp — сборка Win7 падала. go.win7.mod подменяет
// модуль этой папкой (replace); в бинарь она не попадает.
package prettyview
