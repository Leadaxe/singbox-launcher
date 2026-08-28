package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// SPEC 113-F: имя собственного TUN — единственное, что отделяет наш туннель от
// системного WireGuard/AmneziaWG, который теперь законно предлагается в
// аплинки. Ошибка здесь возвращает петлю в выбор.
func TestTunInterfaceNamesReadsOwnTun(t *testing.T) {
	path := writeConfig(t, `{
	  "inbounds": [
	    {"type": "mixed", "listen": "127.0.0.1", "interface_name": "not-a-tun"},
	    {"type": "tun", "interface_name": "singbox-tun0"}
	  ]
	}`)
	names, err := TunInterfaceNames(path)
	if err != nil {
		t.Fatalf("TunInterfaceNames: %v", err)
	}
	if len(names) != 1 || names[0] != "singbox-tun0" {
		t.Fatalf("names = %v, ожидалось ровно имя TUN-инбаунда", names)
	}
}

// TUN без interface_name: имя выбирает само ядро (utunN на macOS), сравнивать
// не с чем. Пустая строка в реестре совпала бы с чем угодно, поэтому её быть
// не должно — интерфейс всё равно ловится по подсети собственного TUN.
func TestTunInterfaceNamesSkipsUnnamedTun(t *testing.T) {
	path := writeConfig(t, `{"inbounds": [{"type": "tun", "auto_route": true}]}`)
	names, err := TunInterfaceNames(path)
	if err != nil {
		t.Fatalf("TunInterfaceNames: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("names = %v, безымянному TUN нечего класть в реестр", names)
	}
}

func TestTunInterfaceNamesWithoutTunInbound(t *testing.T) {
	path := writeConfig(t, `{"inbounds": [{"type": "mixed", "listen": "127.0.0.1"}]}`)
	names, err := TunInterfaceNames(path)
	if err != nil {
		t.Fatalf("TunInterfaceNames: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("names = %v, TUN-инбаундов в конфиге нет", names)
	}
}

// Конфига нет вовсе (первый запуск) — это не повод падать: реестр остаётся
// пустым, а признак «адрес из подсети собственного TUN» продолжает работать.
func TestTunInterfaceNamesMissingFile(t *testing.T) {
	if _, err := TunInterfaceNames(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("ошибка чтения проглочена — вызывающий не отличит пустой реестр от сбоя")
	}
}
