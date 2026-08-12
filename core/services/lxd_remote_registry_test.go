package services

import (
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/internal/lxdclient"
)

// Реестр удалённых демонов — файловый, поэтому проверяем его свойства на
// реальной директории: пустой реестр, изоляция ключей по машинам, атомарность
// перезаписи и честное удаление.

func TestRemoteRegistryEmptyIsNotAnError(t *testing.T) {
	r := NewRemoteRegistry(t.TempDir())
	list, err := r.List()
	if err != nil {
		t.Fatalf("missing registry file must not error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty list, got %d", len(list))
	}
}

// ID разводятся при совпадающих именах: ID — это имя папки с клиентской
// парой, и коллизия означала бы, что две машины делят один сертификат.
func TestRemoteRegistryUniqueIDs(t *testing.T) {
	existing := []RemoteDaemon{{ID: "router"}, {ID: "router-2"}}
	if got := uniqueRemoteID("Router", "10.0.0.1:9500", existing); got != "router-3" {
		t.Errorf("collision resolution: got %q, want router-3", got)
	}
	if got := uniqueRemoteID("", "192.168.10.1:9500", nil); got != "192-168-10-1-9500" {
		t.Errorf("addr fallback slug: got %q", got)
	}
	if got := uniqueRemoteID("Роутер!!!", "", nil); got != "remote" {
		t.Errorf("non-latin name must fall back to %q, got %q", "remote", got)
	}
}

func TestRemoteRegistryCRUD(t *testing.T) {
	dir := t.TempDir()
	r := NewRemoteRegistry(dir)

	// Pair ходит по сети (enroll), поэтому наполняем реестр напрямую —
	// проверяем именно файловый слой.
	r.mu.Lock()
	err := r.saveLocked([]RemoteDaemon{
		{ID: "router", Name: "Router", Addr: "192.168.10.1:9500", ServerFingerprint: "aa11"},
		{ID: "vps", Name: "VPS", Addr: "10.8.0.1:9500", ServerFingerprint: "bb22"},
	})
	r.mu.Unlock()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	list, err := r.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Name != "Router" || list[1].Name != "VPS" {
		t.Fatalf("expected two entries sorted by name, got %+v", list)
	}

	entry, ok, err := r.Get("vps")
	if err != nil || !ok || entry.Addr != "10.8.0.1:9500" {
		t.Fatalf("get vps: entry=%+v ok=%v err=%v", entry, ok, err)
	}

	// Адрес из приглашения может быть listen-адресом (0.0.0.0) — правка
	// адреса не должна трогать ключи.
	if err := r.SetAddr("router", "192.168.10.5:9500"); err != nil {
		t.Fatalf("set addr: %v", err)
	}
	entry, _, _ = r.Get("router")
	if entry.Addr != "192.168.10.5:9500" {
		t.Errorf("addr not updated: %q", entry.Addr)
	}
	if entry.ServerFingerprint != "aa11" {
		t.Errorf("fingerprint must survive addr change, got %q", entry.ServerFingerprint)
	}

	if err := r.SetAddr("nope", "1.2.3.4:1"); err == nil {
		t.Error("SetAddr on unknown id must fail")
	}

	// Удаление стирает и запись, и её ключи.
	identityDir := r.identityDir("router")
	if _, err := lxdclient.LoadOrCreateIdentity(identityDir); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if _, err := os.Stat(identityDir); err != nil {
		t.Fatalf("identity dir must exist before remove: %v", err)
	}
	if err := r.Remove("router"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(identityDir); !os.IsNotExist(err) {
		t.Errorf("identity dir must be gone after remove, err=%v", err)
	}
	list, _ = r.List()
	if len(list) != 1 || list[0].ID != "vps" {
		t.Errorf("after remove want only vps, got %+v", list)
	}
	if err := r.Remove("router"); err == nil {
		t.Error("removing an unknown id must fail")
	}
}

// Каждая машина получает СВОЮ клиентскую пару: сертификат — полный мандат,
// общий ключ означал бы, что отзыв доступа на одном роутере отзывает его везде.
func TestRemoteRegistryIdentityIsolation(t *testing.T) {
	r := NewRemoteRegistry(t.TempDir())
	a, err := lxdclient.LoadOrCreateIdentity(r.identityDir("router"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := lxdclient.LoadOrCreateIdentity(r.identityDir("vps"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint == b.Fingerprint {
		t.Error("each remote must get its own client identity")
	}
	// Повторный вызов для той же машины переиспользует пару, а не плодит новую
	// (иначе на демоне копились бы мусорные клиентские сертификаты).
	again, err := lxdclient.LoadOrCreateIdentity(r.identityDir("router"))
	if err != nil {
		t.Fatal(err)
	}
	if again.Fingerprint != a.Fingerprint {
		t.Error("identity must be stable across loads")
	}
}

// Битый JSON не должен читаться как «нет подключений» — иначе пользователь
// решит, что сопряжения пропали, и повторит enroll (а код одноразовый).
func TestRemoteRegistryCorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	r := NewRemoteRegistry(dir)
	if err := os.MkdirAll(filepath.Dir(r.path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.path(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.List(); err == nil {
		t.Error("corrupt registry must surface an error, not an empty list")
	}
}

// Транспорт к неизвестной машине — ошибка, а не nil-транспорт (иначе
// вызывающий получит панику на первом RPC).
func TestRemoteRegistryTransportUnknownID(t *testing.T) {
	r := NewRemoteRegistry(t.TempDir())
	if _, err := r.Transport("ghost"); err == nil {
		t.Error("Transport for unknown id must fail")
	}
}

// LxdRemoteTransport реализует ProxyTransport и безопасен к Close без
// подключения (UI закрывает транспорт при смене endpoint'а, в том числе до
// первого RPC).
func TestLxdRemoteTransportCloseWithoutDial(t *testing.T) {
	var tr ProxyTransport = NewLxdRemoteTransport(lxdclient.Config{Addr: "127.0.0.1:1"})
	if err := tr.(*LxdRemoteTransport).Close(); err != nil {
		t.Errorf("Close without dial must be a no-op, got %v", err)
	}
}

// SPEC 097, регрессия из эксплуатации: сопряжение с ЧУЖОЙ машиной затирало
// адрес и пин СВОЕГО демона в settings.json (поля-то одни), и лаунчер терял
// связь с локальным ядром — переключение на Local не помогало, потому что
// возвращаться было некуда.
//
// Фикс: не-loopback сопряжение уходит в этот реестр. Здесь фиксируем
// свойства импорта, на которые фикс опирается.
func TestImportPairedDaemon(t *testing.T) {
	dir := t.TempDir()
	r := NewRemoteRegistry(dir)

	// Готовим «уже сопряжённую» identity, как её создаёт локальный pair.
	srcDir := filepath.Join(dir, "bin", "daemon")
	src, err := lxdclient.LoadOrCreateIdentity(srcDir)
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	entry, err := r.ImportPairedDaemon("Router", "192.168.10.1:19091", "AA11BB22", "s3cret", srcDir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if entry.Addr != "192.168.10.1:19091" || entry.Name != "Router" {
		t.Errorf("entry fields: %+v", entry)
	}
	// Отпечаток нормализуется в нижний регистр — пин сравнивается с
	// вычисленным, а тот всегда lowercase hex.
	if entry.ServerFingerprint != "aa11bb22" {
		t.Errorf("fingerprint must be lowercased, got %q", entry.ServerFingerprint)
	}

	// Ключ СКОПИРОВАН, а не переиспользован по ссылке: у записи реестра
	// собственная пара, и удаление записи не трогает исходную.
	imported, err := lxdclient.LoadOrCreateIdentity(r.identityDir(entry.ID))
	if err != nil {
		t.Fatalf("load imported identity: %v", err)
	}
	if imported.Fingerprint != src.Fingerprint {
		t.Errorf("imported identity must be the same trusted key: %s vs %s",
			imported.Fingerprint, src.Fingerprint)
	}
	if err := r.Remove(entry.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcDir, "client_cert.pem")); err != nil {
		t.Errorf("source identity must survive registry removal: %v", err)
	}

	// Повторный импорт того же адреса не плодит дубликаты.
	first, err := r.ImportPairedDaemon("Router", "10.0.0.5:9091", "cc33", "", srcDir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := r.ImportPairedDaemon("Router copy", "10.0.0.5:9091", "cc33", "", srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Errorf("re-import must return the existing entry, got %q vs %q", again.ID, first.ID)
	}
	list, _ := r.List()
	if len(list) != 1 {
		t.Errorf("re-import must not duplicate: %+v", list)
	}

	if _, err := r.ImportPairedDaemon("x", "  ", "", "", srcDir); err == nil {
		t.Error("empty address must be rejected")
	}
}
