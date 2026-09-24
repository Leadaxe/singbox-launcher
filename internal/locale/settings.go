package locale

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// Settings represents the launcher settings stored in bin/settings.json.
type Settings struct {
	Lang string `json:"lang"`
	// PingTestURL — endpoint для Clash GET /proxies/{name}/delay (query url); пусто = не переопредлять дефолт api.
	PingTestURL string `json:"ping_test_url,omitempty"`
	// PingTestAllConcurrency — число параллельных delay-запросов для «test» на вкладке Servers; 0 = не переопредлять.
	PingTestAllConcurrency int `json:"ping_test_all_concurrency,omitempty"`
	// PingTestTimeoutMs — бюджет одиночного url-теста, мс; 0 = дефолт api
	// (api.DefaultPingTestTimeoutMs). Общий для обычного пинга и послойной
	// пробы цепочки: разные бюджеты сделали бы их цифры несравнимыми.
	PingTestTimeoutMs int `json:"ping_test_timeout_ms,omitempty"`
	// SubscriptionAutoUpdateDisabled — пользователь явно выключил автоматическое обновление
	// подписок. По умолчанию (отсутствует / false) — автообновление включено, как раньше.
	// Manual Update всегда работает независимо от флага.
	SubscriptionAutoUpdateDisabled bool `json:"subscription_auto_update_disabled,omitempty"`
	// AutoPingAfterConnectDisabled — выключить автопинг нод через 5с после старта VPN.
	// По умолчанию (отсутствует / false) — автопинг включён. Ручная «test» всегда работает.
	AutoPingAfterConnectDisabled bool `json:"auto_ping_after_connect_disabled,omitempty"`
	// AutoPingAfterConnectMaxProxies — soft cap: если в списке нод больше этого
	// числа, автопинг при connect пропускается (ручная «Test» работает всегда).
	// 0 / отсутствует — использовать встроенный дефолт (services.DefaultAutoPingMaxProxies).
	// Положительный int — переопределить (например, 300 для мощного железа).
	// Поле появилось после field-report на 0.8.7+: на ~500 нод авто-пинг через 5с
	// после connect перегружал TUN-стек и подвешивал игры. См. SPEC 039 §1.3.
	AutoPingAfterConnectMaxProxies int `json:"auto_ping_after_connect_max_proxies,omitempty"`
	// DebugAPIEnabled — пользователь явно включил локальный HTTP debug-API
	// (127.0.0.1:9263 по умолчанию). Off by default.
	DebugAPIEnabled bool `json:"debug_api_enabled,omitempty"`
	// DebugAPIToken — Bearer-токен для debug-API. Генерируется при первом
	// включении, больше не меняется (кроме явной регенерации через UI).
	DebugAPIToken string `json:"debug_api_token,omitempty"`
	// DebugAPIPort — порт для debug-API; 0 / отсутствует означает DefaultPort.
	DebugAPIPort int `json:"debug_api_port,omitempty"`
	// LastTemplateLauncherVersion — версия лаунчера, которая в последний раз
	// успешно скачала bin/wizard_template.json. На старте сравнивается с
	// текущей AppVersion: если меньше → шаблон этой версии скачивается и
	// заменяет протухший (формат шаблона мог разойтись между версиями), старый
	// до того не трогается. См. SPEC 046, core.RefreshTemplateIfStale.
	LastTemplateLauncherVersion string `json:"last_template_launcher_version,omitempty"`
	// LastLocaleLauncherVersion — версия лаунчера, которая в последний раз
	// обновила bin/locale/*.json. Апдейт заменяет только бинарь (bin/
	// сохраняется), поэтому после смены механики ключей (SPEC 111) старый
	// каталог молча давал английский UI. При несовпадении с AppVersion
	// локали докачиваются в фоне один раз — тот же приём, что у шаблона
	// (SPEC 046, LastTemplateLauncherVersion).
	LastLocaleLauncherVersion string `json:"last_locale_launcher_version,omitempty"`

	// LastLauncherVersion / LastCoreVersion — версии, которые лаунчер видел
	// в ПРОШЛЫЙ запуск (SPEC 132, core/maintenance): своя и ядра.
	//
	// Отдельно от LastTemplateLauncherVersion: та отвечает за свежесть
	// шаблона, и чужая логика на ней завела бы одно поле на два несвязанных
	// смысла.
	//
	// Версия ядра — та, что сообщает САМ БИНАРЬ (`sing-box version`): ядро
	// меняется независимо от лаунчера, в том числе подменой файла руками
	// мимо кнопки обновления.
	//
	// Пусто = отметки ещё не было: записывается текущая версия БЕЗ события
	// (объявлять смену не на чем). Смысл отметок — сделать «а что у вас
	// менялось?» видимым в логе релиза, который пишет только WARN; список
	// сервисных работ на этих событиях пока пуст.
	LastLauncherVersion string `json:"last_launcher_version,omitempty"`
	LastCoreVersion     string `json:"last_core_version,omitempty"`

	// ConfigDataRoot — DataDir, с которым config.json собран в последний раз
	// (SPEC 135 §3.5). config.json держит абсолютные пути (.srs, tailscale):
	// после смены корня данных (миграция, portable, переменная окружения,
	// перенос руками) они ложны. На старте несовпадение с текущим DataDir
	// форсирует пересборку (core.RefreshTemplateIfStale); пишется после
	// каждой успешной сборки. Пусто — сборки ещё не было.
	ConfigDataRoot string `json:"config_data_root,omitempty"`

	// FirstRunNoticeShown — одноразовое уведомление «данных предыдущей
	// версии не найдено» (SPEC 135 §3.4) уже показано. Ставится сразу после
	// показа; до тех пор уведомление всплывает на каждом старте, где в
	// DataDir нет state.json и мигрировать нечего.
	FirstRunNoticeShown bool `json:"first_run_notice_shown,omitempty"`

	// HiddenDataNoticeShown — пользователь отказался (Cancel) переключиться
	// на данные, найденные в системном каталоге при включённом portable.txt
	// (SPEC 135, paths.HiddenSystemData). Больше не спрашивать.
	HiddenDataNoticeShown bool `json:"hidden_data_notice_shown,omitempty"`

	// StorageLeftover — что переключатель Portable не смог стереть на
	// старом месте (SPEC 135 §4.2, paths.SwitchReport.Leftover). Пишется в
	// settings.json НОВОГО места сразу после переезда; раздел Storage
	// показывает его строкой, очистка (§4.3) удаляет как остаток переезда.
	// Пусто — остатка нет. Путь абсолютный.
	StorageLeftover string `json:"storage_leftover,omitempty"`

	// HWID — random UUIDv4 идентификатор устройства, отправляемый в
	// `X-Hwid` заголовке при каждом fetch'е подписки. Lazy-generated
	// (EnsureHWID): пустой строкой при первой инсталляции → генерируется и
	// persist'ится на следующем Save. Пользователь может редактировать
	// в Settings tab чтобы перенести HWID между установками (для HWID-binding
	// провайдеров — иначе re-install съест ещё один device slot). См. SPEC 061.
	HWID string `json:"hwid,omitempty"`

	// SubscriptionSendHWID — отключает отправку всех 4 X-Hwid-* заголовков
	// в subscription requests. *bool семантика: nil → дефолт (true, шлём),
	// явный false → выключено. Различение нужно чтобы UI checkbox после
	// первого тика не "забывал" пользовательский выбор. См. SPEC 061 §4.
	SubscriptionSendHWID *bool `json:"subscription_send_hwid,omitempty"`

	// SubscriptionDeviceModelHashed — если true, в `X-Device-Model` уходит
	// sha256(model)[:16] (8 байт hex) вместо raw `MacBookPro18,1`.
	// Провайдер всё ещё видит стабильный device-ID, но не leak'ает hardware
	// family. См. SPEC 061 §4.
	SubscriptionDeviceModelHashed bool `json:"subscription_device_model_hashed,omitempty"`

	// --- Daemon-режим ядра (macOS, lxd) -----------------------------------
	//
	// CoreBackendMode — движок ядра: "" / "classic" — исторический spawn
	// `sing-box run`; "daemon" — управление долгоживущим демоном
	// `sing-box lxd` (gRPC + admin REST). Только macOS.
	CoreBackendMode string `json:"core_backend_mode,omitempty"`
	// DaemonAddress — host:port управляющего канала демона (из приглашения
	// либо дефолт 127.0.0.1:9091 при установке службы лаунчером).
	DaemonAddress string `json:"daemon_address,omitempty"`
	// DaemonServerFingerprint — SHA-256 пин серверного сертификата демона
	// (lowercase hex, из приглашения). Пусто = plain h2c (dev, без TLS).
	DaemonServerFingerprint string `json:"daemon_server_fingerprint,omitempty"`
	// DaemonSecret — Bearer-секрет управляющего канала. Генерируется
	// лаунчером при установке службы; для чужого демона может быть введён
	// вручную. Локальная машина = trust boundary (см. модель state.json),
	// поэтому хранение в settings.json — осознанное решение.
	DaemonSecret string `json:"daemon_secret,omitempty"`
	// DaemonStopVPNOnExit — останавливать ли ядро в демоне при выходе из
	// лаунчера. Default false: выход из лаунчера НЕ трогает VPN — главное
	// UX-преимущество daemon-режима.
	DaemonStopVPNOnExit bool `json:"daemon_stop_vpn_on_exit,omitempty"`
	// DaemonUnsafeNoticeVersion — версия лаунчера, на которой показано
	// модальное предупреждение «служба демона запускает файл, который может
	// подменить пользователь» (SPEC 136 §6). Одно предупреждение на версию:
	// плашка на вкладке LOCAL остаётся до ремонта, модальное окно — нет.
	DaemonUnsafeNoticeVersion string `json:"daemon_unsafe_notice_version,omitempty"`
	// DaemonSystemProxy — метка владения системным прокси в daemon-режиме на
	// Windows (SPEC 141 §7): строка сервера (`http://127.0.0.1:<порт>`),
	// которую лаунчер поставил в WinINet пользователя. «Снять своё» снимает
	// прокси, только если в HKCU стоит ровно она; иначе стирается метка.
	// Пусто — лаунчер прокси не ставил. На macOS прокси ставит ядро.
	DaemonSystemProxy string `json:"daemon_system_proxy,omitempty"`

	// HideAppFromDock — пункт трея «Скрыть из Dock» (macOS). Пишется при
	// каждом переключении пункта, применяется на старте: до этого поля
	// состояние жило только в памяти и терялось при перезапуске (issue #112).
	// На других платформах поле игнорируется (Dock есть только у macOS).
	HideAppFromDock bool `json:"hide_app_from_dock,omitempty"`

	// --- Умолчания подписок (SPEC 118 Т1) ---------------------------------
	//
	// Умолчания reload/max_nodes — поведение лаунчера, одни на все состояния
	// мастера: с v7 они живут здесь, а не в state.json. Первый перенос из
	// старого состояния (миграция v6→v7, шаг 8) заполняет их, только если
	// пользователь ещё не выставил свои.
	//
	// DefaultSubscriptionReload — интервал автообновления подписки по
	// умолчанию (форма прежнего Defaults.Reload, например "4h"); пусто =
	// встроенный дефолт.
	DefaultSubscriptionReload string `json:"default_subscription_reload,omitempty"`
	// DefaultSubscriptionMaxNodes — кап узлов подписки по умолчанию;
	// 0 = встроенный потолок (3000).
	DefaultSubscriptionMaxNodes int `json:"default_subscription_max_nodes,omitempty"`

	// SubscriptionUserAgent — пользовательский User-Agent для subscription
	// requests. Пустая строка / отсутствие поля → fallback на
	// configtypes.BuildSubscriptionUserAgent() (default, например
	// `LxBox/1.1.4 (desktop; macOS)`).
	//
	// Use cases:
	//   - Провайдер требует UA от конкретного клиента (`v2rayN/...`,
	//     `Hiddify/...`, `Shadowrocket/...`) и режет наш default.
	//   - Тестировать обходные конфиги без перекомпиляции лаунчера.
	//   - Облегчить fingerprint когда default UA блокируется на CDN-уровне.
	//
	// Передаётся в fetcher через SubscriptionRequestSettings.UserAgent —
	// applySubscriptionRequestHeaders использует custom если не пустой,
	// иначе BuildSubscriptionUserAgent.
	SubscriptionUserAgent string `json:"subscription_user_agent,omitempty"`
}

// ShouldSendHWID — true если флаг nil (default) или явно true.
// Используется fetcher'ом подписки чтобы решить, добавлять X-Hwid-семейство
// заголовков в request или нет.
func (s *Settings) ShouldSendHWID() bool {
	if s == nil {
		return true
	}
	return s.SubscriptionSendHWID == nil || *s.SubscriptionSendHWID
}

// EnsureHWID возвращает существующий HWID (если уже сгенерирован), либо
// генерирует свежий UUIDv4 и сохраняет в s.HWID. Caller отвечает за
// persistence (SaveSettings) если нужно сохранить новый HWID на диск.
//
// UUIDv4 строится через crypto/rand (RFC 4122 §4.4): 16 random bytes,
// version=4 (bits 12-15 of time_hi_and_version), variant=10 (bits 6-7 of
// clock_seq_hi_and_reserved). Не тянем google/uuid ради 30 строк кода.
func (s *Settings) EnsureHWID() string {
	if s == nil {
		return ""
	}
	if s.HWID != "" {
		return s.HWID
	}
	s.HWID = GenerateUUIDv4()
	return s.HWID
}

// GenerateUUIDv4 generates a fresh random UUIDv4 string in canonical
// 8-4-4-4-12 hex form (lowercase). Used by Settings.EnsureHWID and the
// Settings tab "Regenerate" button.
//
// Falls back to a zero-padded fake UUID if crypto/rand fails (extremely
// unlikely on hosted platforms). Better than panic'ing inside the
// settings-load path.
func GenerateUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		debuglog.WarnLog("locale: crypto/rand for UUIDv4 failed: %v — falling back to zero UUID", err)
		return "00000000-0000-4000-8000-000000000000"
	}
	// RFC 4122 §4.4: version=4 (bits 4-7 of byte 6), variant=10 (bits 6-7 of byte 8).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexBytes := hex.EncodeToString(b[:])
	return hexBytes[0:8] + "-" + hexBytes[8:12] + "-" + hexBytes[12:16] + "-" + hexBytes[16:20] + "-" + hexBytes[20:32]
}

// MarkTemplateInstalled persists the launcher version that just installed the
// template. Called after a successful template download — see SPEC 046.
//
// Failure to persist is non-fatal for the immediate operation but logged: if
// the version isn't recorded the next launch refreshes the template once more
// (cosmetic UX nuisance, not a correctness issue).
func MarkTemplateInstalled(binDir, appVersion string) error {
	s := LoadSettings(binDir)
	if s.LastTemplateLauncherVersion == appVersion {
		return nil
	}
	s.LastTemplateLauncherVersion = appVersion
	return SaveSettings(binDir, s)
}

// MarkConfigDataRoot persists the data root config.json was just built with
// (SPEC 135 §3.5). No write when it is already recorded: rebuilds are frequent.
func MarkConfigDataRoot(binDir, root string) error {
	s := LoadSettings(binDir)
	if s.ConfigDataRoot == root {
		return nil
	}
	s.ConfigDataRoot = root
	return SaveSettings(binDir, s)
}

// MarkFirstRunNoticeShown persists that the one-time "no previous data found"
// notice was shown (SPEC 135 §3.4).
func MarkFirstRunNoticeShown(binDir string) error {
	s := LoadSettings(binDir)
	if s.FirstRunNoticeShown {
		return nil
	}
	s.FirstRunNoticeShown = true
	return SaveSettings(binDir, s)
}

// MarkHiddenDataNoticeShown persists that the user declined to switch to the
// data found in the system folder while portable.txt is present (SPEC 135).
func MarkHiddenDataNoticeShown(binDir string) error {
	s := LoadSettings(binDir)
	if s.HiddenDataNoticeShown {
		return nil
	}
	s.HiddenDataNoticeShown = true
	return SaveSettings(binDir, s)
}

// MarkDaemonUnsafeNoticeShown persists the launcher version that showed the
// "daemon service runs a user-writable binary" warning (SPEC 136 §6).
func MarkDaemonUnsafeNoticeShown(binDir, appVersion string) error {
	s := LoadSettings(binDir)
	if s.DaemonUnsafeNoticeVersion == appVersion {
		return nil
	}
	s.DaemonUnsafeNoticeVersion = appVersion
	return SaveSettings(binDir, s)
}

// MarkStorageLeftover persists what a Portable switch left behind at the old
// place ("" clears it) into settings.json of the new place (SPEC 135 §4.2).
func MarkStorageLeftover(binDir, path string) error {
	s := LoadSettings(binDir)
	if s.StorageLeftover == path {
		return nil
	}
	s.StorageLeftover = path
	return SaveSettings(binDir, s)
}

// MarkLocalesRefreshed persists the launcher version that just refreshed the
// locale catalogs. Not marked on download failure — the next launch retries.
func MarkLocalesRefreshed(binDir, appVersion string) error {
	s := LoadSettings(binDir)
	if s.LastLocaleLauncherVersion == appVersion {
		return nil
	}
	s.LastLocaleLauncherVersion = appVersion
	return SaveSettings(binDir, s)
}

// LoadSettings reads settings from binDir/settings.json.
// Returns default settings if file doesn't exist or is invalid.
func LoadSettings(binDir string) Settings {
	s := Settings{Lang: "en"}
	path := filepath.Join(binDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		debuglog.WarnLog("locale: failed to parse settings.json: %v", err)
		return Settings{Lang: "en"}
	}
	if s.Lang == "" {
		s.Lang = "en"
	}
	return s
}

// SaveSettings writes settings to binDir/settings.json.
//
// Writes are atomic: we stage to a sibling temp file then rename over the
// real one. Protects against power loss or a crash mid-write leaving the
// user with a zero-byte settings.json and losing language / ping / subs
// preferences on next launch.
func SaveSettings(binDir string, s Settings) error {
	path := filepath.Join(binDir, "settings.json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("locale: marshal settings: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, platform.DefaultFileMode); err != nil {
		return fmt.Errorf("locale: write temp settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("locale: rename settings: %w", err)
	}
	return nil
}
