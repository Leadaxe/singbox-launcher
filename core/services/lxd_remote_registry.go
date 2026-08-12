package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// Реестр УДАЛЁННЫХ демонов `sing-box lxd` (SPEC 097).
//
// Отличие от локального сопряжения (core/daemon_manager_darwin.go): там ровно
// один демон, которым лаунчер сам управляет на этой машине, и его адрес/пин
// лежат в settings.json единственным набором полей. Здесь машин может быть
// несколько (роутер, VPS, домашний mac), они переживают перезапуск и
// выбираются пользователем — поэтому отдельный файл-реестр и своя папка
// клиентских ключей.
//
// Никакой платформенной специфики: лаунчер на Windows управляет
// linux-роутером. Демонный ДВИЖОК (запуск своего демона) остаётся macOS-only —
// это про другое.

// RemoteDaemon — сохранённое подключение к удалённой машине.
type RemoteDaemon struct {
	// ID — стабильный идентификатор записи (slug имени + суффикс при
	// коллизии). Он же — имя папки с клиентской парой.
	ID string `json:"id"`
	// Name — человекочитаемое имя («Роутер RouteRich»).
	Name string `json:"name"`
	// Addr — host:port управляющего канала демона.
	Addr string `json:"addr"`
	// ServerFingerprint — SHA-256 пин серверного сертификата (lowercase hex).
	// Пусто = plain h2c (dev-демон на loopback); для сети обязателен.
	ServerFingerprint string `json:"server_fingerprint,omitempty"`
	// Secret — Bearer-секрет; нужен только plain-h2c демону. При mTLS
	// мандатом служит клиентский сертификат.
	Secret string `json:"secret,omitempty"`
	// GOOS/GOARCH — платформа и архитектура МАШИНЫ (SPEC 098 §2.4).
	//
	// Живут здесь, а не в состоянии визарда, потому что это свойство самой
	// машины, а не одной из её настроек: строка списка показывает их, визард
	// читает их, генерация собирает из них TargetSpec. Один источник правды —
	// иначе остаётся способ разъехаться: собрать конфиг под архитектуру,
	// отличную от той, что показана в списке.
	//
	// Пустые значения трактуются как linux/amd64 (см. TargetSpec): самый
	// частый случай для роутера и VPS.
	GOOS   string `json:"goos,omitempty"`
	GOARCH string `json:"goarch,omitempty"`
	// StateDir — АБСОЛЮТНЫЙ state-каталог демона на его машине; кешируется из
	// `GET /admin/info` при каждом успешном соединении (SPEC 063).
	//
	// Нужен генерации: путь `<StateDir>/resources/<name>` уезжает в
	// rule_set[].path, и резолвит его ядро НА ТОЙ СТОРОНЕ. Кешируем, чтобы
	// сборка конфига не требовала живой сети — Configure должен работать и с
	// выключенным роутером.
	StateDir string `json:"state_dir,omitempty"`
	// AddedAt — когда сопряглись (RFC3339, для UI-списка).
	AddedAt string `json:"added_at,omitempty"`
}

// remoteRegistryFile — <bin>/remote-daemons.json.
const remoteRegistryFile = "remote-daemons.json"

// RemoteRegistry — файловый реестр удалённых демонов.
//
// Файл читается/пишется целиком: записей единицы, а атомарная перезапись
// проще и надёжнее частичных апдейтов.
type RemoteRegistry struct {
	execDir string
	mu      sync.Mutex
}

// NewRemoteRegistry создаёт реестр, живущий в <execDir>/bin/.
func NewRemoteRegistry(execDir string) *RemoteRegistry {
	return &RemoteRegistry{execDir: execDir}
}

func (r *RemoteRegistry) path() string {
	return filepath.Join(platform.GetBinDir(r.execDir), remoteRegistryFile)
}

// identityDir — папка клиентской пары для конкретной удалённой машины.
// У каждой машины своя: сертификат = полный мандат, и общий ключ на все
// устройства означал бы, что отзыв доступа на одном роутере отзывает его
// везде.
func (r *RemoteRegistry) identityDir(id string) string {
	return filepath.Join(platform.GetBinDir(r.execDir), "remote-daemons", id)
}

// List возвращает сохранённые подключения, отсортированные по имени.
// Отсутствующий файл — не ошибка (пустой список).
func (r *RemoteRegistry) List() ([]RemoteDaemon, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listLocked()
}

func (r *RemoteRegistry) listLocked() ([]RemoteDaemon, error) {
	raw, err := os.ReadFile(r.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("remote registry: read: %w", err)
	}
	var out []RemoteDaemon
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("remote registry: parse %s: %w", r.path(), err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *RemoteRegistry) saveLocked(list []RemoteDaemon) error {
	binDir := platform.GetBinDir(r.execDir)
	if err := os.MkdirAll(binDir, platform.DefaultDirMode); err != nil {
		return fmt.Errorf("remote registry: mkdir: %w", err)
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	// Атомарно: tmp + rename, иначе обрыв записи оставит битый JSON и
	// пользователь потеряет ВСЕ сопряжения разом.
	tmp := r.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, platform.DefaultFileMode); err != nil {
		return fmt.Errorf("remote registry: write: %w", err)
	}
	if err := os.Rename(tmp, r.path()); err != nil {
		return fmt.Errorf("remote registry: rename: %w", err)
	}
	return nil
}

// Get возвращает запись по ID.
func (r *RemoteRegistry) Get(id string) (RemoteDaemon, bool, error) {
	list, err := r.List()
	if err != nil {
		return RemoteDaemon{}, false, err
	}
	for _, d := range list {
		if d.ID == id {
			return d, true, nil
		}
	}
	return RemoteDaemon{}, false, nil
}

// Pair сопрягается с удалённым демоном по приглашению `адрес#отпечаток#код`
// и сохраняет подключение под именем name.
//
// Код приглашения одноразовый и сгорает после первого enroll, поэтому запись
// в реестр делается ТОЛЬКО после успешного enroll: иначе при ошибке сети мы
// сохранили бы подключение, которым уже нельзя воспользоваться повторно.
func (r *RemoteRegistry) Pair(inviteRaw, name, secret string) (RemoteDaemon, error) {
	return r.PairWithAddr(inviteRaw, name, "", secret)
}

// PairWithAddr — сопряжение с явным адресом подключения (SPEC 097).
//
// Демон печатает в приглашении СВОЙ listen-адрес. При listen 0.0.0.0 оттуда
// приезжает нерабочее значение, а при listen на LAN-интерфейсе — адрес,
// по которому мы можем быть недоступны. Пользователь указывает адрес, по
// которому реально достучится; пустой addr = взять из приглашения.
//
// Enroll всё равно идёт на АДРЕС ПОЛЬЗОВАТЕЛЯ: сопрягаться по одному адресу,
// а работать по другому — верный способ получить «сопряглись, но не
// подключается».
func (r *RemoteRegistry) PairWithAddr(inviteRaw, name, addr, secret string) (RemoteDaemon, error) {
	invite, err := lxdclient.ParseInvite(inviteRaw)
	if err != nil {
		return RemoteDaemon{}, err
	}
	if a := strings.TrimSpace(addr); a != "" {
		if _, _, splitErr := net.SplitHostPort(a); splitErr != nil {
			return RemoteDaemon{}, fmt.Errorf("address %q is not a valid host:port: %w", a, splitErr)
		}
		invite.Addr = a
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	list, err := r.listLocked()
	if err != nil {
		return RemoteDaemon{}, err
	}
	id := uniqueRemoteID(name, invite.Addr, list)

	identity, err := lxdclient.LoadOrCreateIdentity(r.identityDir(id))
	if err != nil {
		return RemoteDaemon{}, fmt.Errorf("remote pair: identity: %w", err)
	}
	client := lxdclient.New(lxdclient.Config{
		Addr:              invite.Addr,
		ServerFingerprint: invite.ServerFingerprint,
		Identity:          identity,
		Secret:            secret,
	})
	if err := client.Enroll(invite.Code, "singbox-launcher"); err != nil {
		// Ключи оставляем: повторная попытка с новым кодом переиспользует их,
		// и на демоне не появится второй мусорный клиентский сертификат.
		return RemoteDaemon{}, fmt.Errorf("remote pair: enroll at %s: %w", invite.Addr, err)
	}

	entry := RemoteDaemon{
		ID:                id,
		Name:              strings.TrimSpace(name),
		Addr:              invite.Addr,
		ServerFingerprint: invite.ServerFingerprint,
		Secret:            secret,
		AddedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	if entry.Name == "" {
		entry.Name = invite.Addr
	}
	if err := r.saveLocked(append(list, entry)); err != nil {
		return RemoteDaemon{}, err
	}
	debuglog.InfoLog("remote pair: enrolled %q at %s", entry.Name, entry.Addr)
	return entry, nil
}

// Update меняет имя и адрес записи, не трогая ключи и пин.
//
// ID (он же папка ключей) НЕ переименовывается вслед за именем: ключ уже
// доверен демоном, и переезд папки означал бы потерю сопряжения ради
// косметики.
func (r *RemoteRegistry) Update(id, name, addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("remote registry: empty address")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("address %q is not a valid host:port: %w", addr, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	list, err := r.listLocked()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID != id {
			continue
		}
		if n := strings.TrimSpace(name); n != "" {
			list[i].Name = n
		}
		list[i].Addr = addr
		return r.saveLocked(list)
	}
	return fmt.Errorf("remote registry: unknown id %q", id)
}

// SetAddr меняет адрес сохранённого подключения, не трогая ключи.
//
// Нужно, потому что в приглашении стоит listen-адрес демона: у роутера с
// listen 0.0.0.0 приглашение принесёт нерабочий адрес, и пользователь
// правит его на реальный LAN-адрес после сопряжения.
func (r *RemoteRegistry) SetAddr(id, addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list, err := r.listLocked()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID != id {
			continue
		}
		list[i].Addr = strings.TrimSpace(addr)
		return r.saveLocked(list)
	}
	return fmt.Errorf("remote registry: unknown id %q", id)
}

// Remove удаляет подключение, его клиентские ключи и всё её имущество:
// состояние визарда, снапшоты, собранный конфиг, .srs и тела подписок
// (SPEC 098 §3.1.9).
//
// Две директории — ровно потому, что SPEC 098 §5.6 держит всё имущество
// машины под одним корнем. Пока конфиг лежал в общем bin/remote-config.json,
// а .srs в общем bin/rule-sets/, удаление машины было поиском следов по bin/
// с риском задеть чужие файлы.
//
// Регистрация на СТОРОНЕ демона остаётся — снять её может только сам демон
// (`sing-box lxd client remove`). Мы честно забываем ключ у себя, а не
// делаем вид, что отозвали доступ.
func (r *RemoteRegistry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list, err := r.listLocked()
	if err != nil {
		return err
	}
	out := make([]RemoteDaemon, 0, len(list))
	found := false
	for _, d := range list {
		if d.ID == id {
			found = true
			continue
		}
		out = append(out, d)
	}
	if !found {
		return fmt.Errorf("remote registry: unknown id %q", id)
	}
	if err := r.saveLocked(out); err != nil {
		return err
	}
	if err := os.RemoveAll(r.identityDir(id)); err != nil {
		debuglog.WarnLog("remote registry: remove identity dir for %q: %v", id, err)
	}
	// Пустой id снёс бы плоскую remote/ со всеми машинами — но пустой id не
	// проходит validation при добавлении, так что сюда он попасть не может.
	// Проверка стоит на случай, если запись попала в реестр правкой файла.
	if strings.TrimSpace(id) != "" {
		if err := os.RemoveAll(platform.GetRemoteMachineDir(r.execDir, id)); err != nil {
			debuglog.WarnLog("remote registry: remove state dir for %q: %v", id, err)
		}
	}
	return nil
}

// SetPlatform фиксирует платформу и архитектуру машины (SPEC 098 §2.4).
//
// Платформа — свойство машины, а не настройка конфига: под неё собирается
// config.json, и она же показывается в строке списка. Держать её в состоянии
// визарда значило бы иметь два источника правды и способ разъехаться —
// собрать под архитектуру, отличную от показанной.
func (r *RemoteRegistry) SetPlatform(id, goos, goarch string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list, err := r.listLocked()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID != id {
			continue
		}
		list[i].GOOS = strings.TrimSpace(goos)
		list[i].GOARCH = strings.TrimSpace(goarch)
		return r.saveLocked(list)
	}
	return fmt.Errorf("remote registry: unknown id %q", id)
}

// SetStateDir кеширует state-каталог демона, полученный из /admin/info.
//
// Тихо no-op при пустом значении и при совпадении: зовётся на каждом
// health-опросе, и перезаписывать файл реестра ради того же самого не нужно.
func (r *RemoteRegistry) SetStateDir(id, stateDir string) error {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	list, err := r.listLocked()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID != id {
			continue
		}
		if list[i].StateDir == stateDir {
			return nil
		}
		list[i].StateDir = stateDir
		return r.saveLocked(list)
	}
	return fmt.Errorf("remote registry: unknown id %q", id)
}

// ResourceDir — каталог ресурсов машины: `<state_dir>/resources` (SPEC 063).
// Пусто, если state_dir ещё не известен (ни одного успешного соединения).
func (d RemoteDaemon) ResourceDir() string {
	if s := strings.TrimSpace(d.StateDir); s != "" {
		return s + "/resources"
	}
	return ""
}

// Target собирает TargetSpec генерации из записи реестра (SPEC 098 §2.4).
// Единственный санкционированный способ узнать, под что собирать конфиг
// машины.
//
// Пустая платформа = linux/amd64, а НЕ платформа хоста: запись без явного
// GOOS осталась от сопряжения до SPEC 098, и удалённая машина в этом проекте —
// почти всегда роутер или VPS. Подставить сюда runtime.GOOS значило бы на
// macOS-лаунчере молча собрать darwin-конфиг для linux-роутера.
func (d RemoteDaemon) Target() wizardtemplate.TargetSpec {
	goos, goarch := strings.TrimSpace(d.GOOS), strings.TrimSpace(d.GOARCH)
	if goos == "" {
		goos = "linux"
	}
	if goarch == "" {
		goarch = "amd64"
	}
	return wizardtemplate.RemoteTarget(goos, goarch)
}

// ImportPairedDaemon добавляет в реестр УЖЕ сопряжённое подключение —
// без enroll (SPEC 097).
//
// Зачем: сопряжение с демоном (адрес + пин + клиентский ключ) до этой спеки
// делалось единственным путём — PairDaemonWithInvite, который пишет в
// settings.json. Если сопрягались с чужой машиной, её данные затирали
// подключение к своему демону: поля-то одни. Импорт разрывает эту связь —
// реестр забирает подключение себе, а settings.json можно вернуть локальному
// демону.
//
// identitySrcDir — папка, откуда скопировать клиентскую пару (для
// импортируемого сопряжения это bin/daemon/). Ключ копируется, а не
// переиспользуется по ссылке: у каждой записи реестра свои ключи, иначе
// отзыв доступа на одной машине отзывал бы его на всех.
func (r *RemoteRegistry) ImportPairedDaemon(name, addr, fingerprint, secret, identitySrcDir string) (RemoteDaemon, error) {
	if strings.TrimSpace(addr) == "" {
		return RemoteDaemon{}, fmt.Errorf("remote import: empty address")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	list, err := r.listLocked()
	if err != nil {
		return RemoteDaemon{}, err
	}
	// Уже импортировано (тот же адрес) — возвращаем существующую запись,
	// чтобы повторный импорт не плодил дубликаты.
	for _, d := range list {
		if strings.EqualFold(d.Addr, strings.TrimSpace(addr)) {
			return d, nil
		}
	}

	id := uniqueRemoteID(name, addr, list)
	if strings.TrimSpace(identitySrcDir) != "" {
		if err := copyIdentity(identitySrcDir, r.identityDir(id)); err != nil {
			return RemoteDaemon{}, fmt.Errorf("remote import: copy identity: %w", err)
		}
	}
	entry := RemoteDaemon{
		ID:                id,
		Name:              strings.TrimSpace(name),
		Addr:              strings.TrimSpace(addr),
		ServerFingerprint: strings.ToLower(strings.TrimSpace(fingerprint)),
		Secret:            secret,
		AddedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	if entry.Name == "" {
		entry.Name = entry.Addr
	}
	if err := r.saveLocked(append(list, entry)); err != nil {
		return RemoteDaemon{}, err
	}
	debuglog.InfoLog("remote import: %q at %s taken into the registry", entry.Name, entry.Addr)
	return entry, nil
}

// copyIdentity копирует клиентскую пару (cert+key) из src в dst.
func copyIdentity(src, dst string) error {
	if err := os.MkdirAll(dst, platform.DefaultDirMode); err != nil {
		return err
	}
	for _, name := range []string{"client_cert.pem", "client_key.pem"} {
		raw, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			return err
		}
		// 0600: приватный ключ клиента = полный мандат на демон.
		if err := os.WriteFile(filepath.Join(dst, name), raw, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// RemoteHealth — состояние удалённой машины для строки списка (SPEC 097).
//
// Диагностика КАЖДОЙ машины отдельно: до этого окно показывало статус
// локального демона под заголовком «Remote», то есть данные не той машины,
// на которую смотрит пользователь.
type RemoteHealth struct {
	// Reachable — ответила ли машина на /admin/status.
	Reachable bool
	// Err — почему не ответила (для подсказки в UI).
	Err string
	// CoreStatus — idle | started | fatal.
	CoreStatus string
	// LastError — последняя ошибка применения конфига на той стороне.
	LastError string
	// Version / StateDir — паспорт демона (/admin/info); best-effort.
	Version  string
	StateDir string
	// ActiveSHA / LastGoodSHA — хеши работающего и последнего удачного
	// конфигов. По ним видно, действительно ли на машине крутится то, что
	// мы задеплоили: совпадение с хешем нашего файла — единственная честная
	// проверка «доехало».
	ActiveSHA   string
	LastGoodSHA string
	// InterruptedApply — предыдущее применение конфига прервалось (демон
	// упал или его убили в процессе). Ядро при этом работает на last-good.
	InterruptedApply bool
}

// Health опрашивает машину: статус ядра + паспорт демона.
//
// Блокирующий вызов по сети — вызывающий обязан звать из горутины, иначе
// недоступный роутер подвесит UI на таймаут REST-клиента.
func (r *RemoteRegistry) Health(id string) RemoteHealth {
	client, err := r.adminClient(id)
	if err != nil {
		return RemoteHealth{Err: err.Error()}
	}
	status, err := client.Status()
	if err != nil {
		return RemoteHealth{Err: err.Error()}
	}
	out := RemoteHealth{
		Reachable:        true,
		CoreStatus:       status.Status,
		LastError:        status.LastError,
		ActiveSHA:        status.ActiveSHA,
		LastGoodSHA:      status.LastGoodSHA,
		InterruptedApply: status.InterruptedApply,
	}
	// Паспорт — best-effort: машина уже отвечает, и отсутствие /admin/info
	// (старый демон) не повод считать её недоступной.
	if info, infoErr := client.Info(); infoErr == nil {
		out.Version = info.Version
		out.StateDir = info.StateDir
		// Кешируем в реестр: генерация конфига берёт отсюда путь ресурс-стора
		// и не зависит от того, доступна ли машина в этот момент.
		if err := r.SetStateDir(id, info.StateDir); err != nil {
			debuglog.WarnLog("remote registry: cache state_dir for %q: %v", id, err)
		}
	}
	return out
}

// StartCore / StopCore — запуск и остановка ЯДРА на удалённой машине
// (SPEC 097).
//
// Демон переживает обе операции: он держит ядро внутри себя, а управляющий
// канал остаётся. Останов ядра рвёт VPN у всех, кто ходит через эту машину,
// поэтому вызывающий обязан спросить подтверждение.
//
// Блокирующие сетевые вызовы — звать из горутины.
func (r *RemoteRegistry) StartCore(id string) error {
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	if err := client.Start(); err != nil {
		return fmt.Errorf("remote start: %w", err)
	}
	return nil
}

func (r *RemoteRegistry) StopCore(id string) error {
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	if err := client.Stop(); err != nil {
		return fmt.Errorf("remote stop: %w", err)
	}
	return nil
}

// SyncResources заливает на машину rule-set'ы, на которые ссылается её
// конфиг (SPEC 063 форка ядра).
//
// Зачем: конфиг ссылается на `<state_dir>/resources/<name>`, и без этого шага
// ядро на той стороне не найдёт файл — apply пройдёт, а инстанс не
// поднимется. Порядок обязателен: сначала ресурсы, потом конфиг.
//
// Гоняет только изменённое: демон отдаёт sha256 каждого имени, и совпадающие
// пропускаются. На больших geo-базах это разница между «моментально» и
// «десятки мегабайт по Wi-Fi на каждый Deploy».
//
// 409 на PUT означает, что имя занято ссылкой из активного или last-good
// конфига. Это не сбой: файл там уже такой, какой нужен (иначе хеш бы не
// совпал и мы бы сюда не дошли), поэтому просто пропускаем — переписать его
// демон и не должен, пока на него ссылаются.
//
// Блокирующие сетевые вызовы — звать из горутины.
func (r *RemoteRegistry) SyncResources(id string, files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	remote, err := client.Resources()
	if err != nil {
		return fmt.Errorf("remote resources: list: %w", err)
	}
	have := make(map[string]string, len(remote))
	for _, res := range remote {
		have[res.Name] = strings.ToLower(res.SHA256)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names) // детерминированный порядок — читаемые логи

	uploaded := 0
	for _, name := range names {
		body := files[name]
		sum := sha256.Sum256(body)
		if have[name] == hex.EncodeToString(sum[:]) {
			continue // на машине уже ровно этот файл
		}
		if _, putErr := client.PutResource(name, body); putErr != nil {
			var resErr *lxdclient.ResourceError
			if errors.As(putErr, &resErr) && resErr.InUse() {
				debuglog.DebugLog("remote resources: %q is referenced by a live config, keeping it", name)
				continue
			}
			return fmt.Errorf("remote resources: upload %q: %w", name, putErr)
		}
		uploaded++
	}
	if uploaded > 0 {
		debuglog.InfoLog("remote resources: uploaded %d file(s) to %q", uploaded, id)
	}
	return nil
}

// ApplyConfig отправляет конфиг на удалённую машину (SPEC 097).
//
// Демон валидирует конфиг сабпроцессом ДО подмены инстанса и откатывается
// на last-good, если новый не стартовал: неудачный деплой не оставит роутер
// без VPN. Ошибка 422 (ApplyError.Rejected) означает, что конфиг забракован
// и работающий инстанс не тронут вовсе.
//
// Блокирующий сетевой вызов — звать из горутины.
func (r *RemoteRegistry) ApplyConfig(id string, config []byte) error {
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	if err := client.Apply(config); err != nil {
		return fmt.Errorf("remote apply: %w", err)
	}
	debuglog.InfoLog("remote apply: config delivered to %q (%d bytes)", id, len(config))
	return nil
}

// adminClient собирает REST-клиента к записи реестра (адрес + пин + ключ).
func (r *RemoteRegistry) adminClient(id string) (*lxdclient.Client, error) {
	entry, ok, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("remote registry: unknown id %q", id)
	}
	cfg := lxdclient.Config{
		Addr:              entry.Addr,
		ServerFingerprint: entry.ServerFingerprint,
		Secret:            entry.Secret,
	}
	if cfg.TLSEnabled() {
		identity, idErr := lxdclient.LoadOrCreateIdentity(r.identityDir(id))
		if idErr != nil {
			return nil, fmt.Errorf("remote registry: identity for %q: %w", id, idErr)
		}
		cfg.Identity = identity
	}
	return lxdclient.New(cfg), nil
}

// Transport строит ProxyTransport к сохранённому подключению.
// Вызывающий отвечает за Close, когда транспорт больше не нужен.
func (r *RemoteRegistry) Transport(id string) (*LxdRemoteTransport, error) {
	entry, ok, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("remote registry: unknown id %q", id)
	}
	cfg := lxdclient.Config{
		Addr:              entry.Addr,
		ServerFingerprint: entry.ServerFingerprint,
		Secret:            entry.Secret,
	}
	// Identity нужна только TLS-каналу; plain-h2c демон (dev на loopback)
	// авторизует Bearer-секретом.
	if cfg.TLSEnabled() {
		identity, err := lxdclient.LoadOrCreateIdentity(r.identityDir(id))
		if err != nil {
			return nil, fmt.Errorf("remote registry: identity for %q: %w", id, err)
		}
		cfg.Identity = identity
	}
	return NewLxdRemoteTransport(cfg), nil
}

// uniqueRemoteID делает slug из имени (или адреса) и разводит коллизии
// суффиксом. ID — это имя папки с ключами, поэтому только [a-z0-9_-].
func uniqueRemoteID(name, addr string, existing []RemoteDaemon) string {
	base := slugifyRemote(name)
	if base == "" {
		base = slugifyRemote(addr)
	}
	if base == "" {
		base = "remote"
	}
	taken := make(map[string]bool, len(existing))
	for _, d := range existing {
		taken[d.ID] = true
	}
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken[candidate] {
			return candidate
		}
	}
}

func slugifyRemote(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ', r == '.', r == ':':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
