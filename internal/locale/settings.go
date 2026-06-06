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
	// текущей AppVersion: если меньше → шаблон удаляется как протухший
	// (формат шаблона мог разойтись между версиями). См. SPEC 046.
	LastTemplateLauncherVersion string `json:"last_template_launcher_version,omitempty"`

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

	// SubscriptionUserAgent — пользовательский User-Agent для subscription
	// requests. Пустая строка / отсутствие поля → fallback на
	// configtypes.BuildSubscriptionUserAgent() (default, например
	// `singbox-launcher/0.9.9 (macOS arm64)`).
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
// the version isn't recorded the next launcher upgrade will re-invalidate the
// template once more (cosmetic UX nuisance, not a correctness issue).
func MarkTemplateInstalled(binDir, appVersion string) error {
	s := LoadSettings(binDir)
	if s.LastTemplateLauncherVersion == appVersion {
		return nil
	}
	s.LastTemplateLauncherVersion = appVersion
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
