// File endpoint_schemes.go — «эта схема живёт в endpoints[], а не в
// outbounds[]» (SPEC 122 §2.1) и раскладка каталога состояния tailnet.
//
// Признак endpoint'а — поле `kind` протокола в реестре
// (contract/registry/protocols/<scheme>.json: `endpoint` против `outbound`),
// читается движком реестра; своей таблицы схем здесь нет (SPEC 142 A9).
package config

import (
	"path/filepath"
	"strings"
	"sync"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/registry"
)

// IsEndpointScheme — уедет ли узел этой схемы в endpoints[]: `kind` её
// протокола в реестре — endpoint.
func IsEndpointScheme(scheme string) bool {
	return registry.MustGet().ProtocolKind(scheme) == registry.KindEndpoint
}

// SchemeTailscale — схема узла tailnet (contract/registry/protocols/tailscale.json).
// Алиас константы модели: предикат IsExitCapable живёт в configtypes
// (leaf-пакет, импортировать config он не может), и строка обязана быть одна.
const SchemeTailscale = configtypes.SchemeTailscale

// tailscaleStateDirRoot — каталог, ВНУТРИ которого лежат каталоги состояния
// узлов tailnet. Ставится приложением (core.AppController) тем же приёмом,
// что и хуки проб: пакет config не знает ни про DataDir, ни про файловый
// сервис, а эмиссия одна и та же для сборки и для превью вкладки JSON.
//
// Пусто = состояние ставить некуда (превью, тесты): поле не подставляется
// вовсе, и ядро возьмёт свой дефолт. Молча писать относительный путь нельзя
// — рабочий каталог ядра лаунчеру не принадлежит.
var (
	tailscaleStateDirRoot   string
	tailscaleStateDirRootMu sync.RWMutex
)

// SetTailscaleStateDirRoot задаёт корень каталогов состояния tailnet.
// Ожидается абсолютный путь вида `<DataDir>/bin/tailscale` — тот же корень
// `<DataDir>/bin`, относительно которого эмитятся локальные .srs
// (build.CollectSrsCachedPaths).
func SetTailscaleStateDirRoot(root string) {
	root = strings.TrimSpace(root)
	if root != "" {
		root = filepath.Clean(root)
	}
	tailscaleStateDirRootMu.Lock()
	tailscaleStateDirRoot = root
	tailscaleStateDirRootMu.Unlock()
}

// TailscaleStateDirRoot — текущий ЛОКАЛЬНЫЙ корень (для тестов, GC и
// диагностики).
func TailscaleStateDirRoot() string {
	tailscaleStateDirRootMu.RLock()
	defer tailscaleStateDirRootMu.RUnlock()
	return tailscaleStateDirRoot
}

// applyTailscaleStateDirectory подставляет state_directory узлу tailnet.
//
// Только если ключа В ТЕЛЕ НЕТ: явное значение пользователя — решение
// пользователя, и перебивать его эмиттер не вправе. Дефолт ядра — общий
// каталог `tailscale` относительно рабочего каталога процесса, поэтому два
// узла tailnet без этого поля сели бы в одно состояние и второй перетёр бы
// идентичность первого.
//
// remoteRoot — корень НА МАШИНЕ-ИСПОЛНИТЕЛЕ (`<state_dir>/tailscale` её
// демона, SPEC 063/122). Приходит параметром от сборки цели
// (ParserConfig.TailscaleRemoteStateDirRoot), а не из глобала: окна
// Конфигуратора разных целей открыты одновременно (SPEC 178), и общий корень
// унёс бы путь роутера в локальный конфиг. Непустой = конфиг собирается для
// чужой машины, и локальный корень
// в него писать нельзя: путь резолвит ядро на той стороне, нашего пути там
// нет. Ядро создало бы его от своего корня, и состояние узла оседало бы в
// каталоге вида `/Applications/…/bin/tailscale/<тег>` на роутере — рабочем,
// но абсурдном и сносимом первой же чисткой overlay.
func applyTailscaleStateDirectory(endpoint map[string]interface{}, scheme, tag, remoteRoot string) {
	if scheme != SchemeTailscale || endpoint == nil {
		return
	}
	remoteRoot = strings.TrimSpace(remoteRoot)
	defer applyTailscaleTaildropDirectory(endpoint, remoteRoot != "")
	applyTailscaleTaildropDefault(endpoint)
	if _, present := endpoint["state_directory"]; present {
		return
	}
	if remoteRoot != "" {
		// Разделитель "/" литералом: путь ЧУЖОЙ машины, и filepath.Join на
		// Windows-лаунчере дал бы обратные слэши в пути linux-роутера.
		endpoint["state_directory"] = strings.TrimRight(remoteRoot, "/") + "/" + sanitizeStateDirName(tag)
		return
	}
	root := TailscaleStateDirRoot()
	if root == "" {
		return
	}
	// filepath.Join, а не конкатенация через "/": на Windows корень приходит
	// с обратными слэшами, и смешивать разделители в одном пути незачем.
	endpoint["state_directory"] = filepath.Join(root, sanitizeStateDirName(tag))
}

// applyTailscaleTaildropDefault — «Allow Taildrop» по умолчанию выключено
// (контракт 1.1.150, решение владельца 02.11.2026): disable_taildrop: true,
// если в теле нет явного значения. Дефолт ядра — включено, поэтому выключает
// сборка. Ядро старше min_core поля не знает — полевой гейт config-формы
// снимает его вместе с остальными, и Taildrop там остаётся как у ядра.
func applyTailscaleTaildropDefault(endpoint map[string]interface{}) {
	if _, present := endpoint["disable_taildrop"]; present {
		return
	}
	endpoint["disable_taildrop"] = true
}

// applyTailscaleTaildropDirectory подставляет taildrop_directory =
// <state_directory>/taildrop (SPEC 175 §2), только если ключа в теле нет.
// Дефолт ядра — общий `Taildrop` от рабочего каталога: входящие всех узлов
// tailnet легли бы в одну папку. Внутри каталога состояния входящие
// переименовываются, удаляются и переносятся демоном вместе с узлом.
// remote — путь чужой машины, разделитель "/".
func applyTailscaleTaildropDirectory(endpoint map[string]interface{}, remote bool) {
	if _, present := endpoint["taildrop_directory"]; present {
		return
	}
	dir, _ := endpoint["state_directory"].(string)
	if dir = strings.TrimSpace(dir); dir == "" {
		return
	}
	if remote {
		endpoint["taildrop_directory"] = strings.TrimRight(dir, "/") + "/taildrop"
		return
	}
	endpoint["taildrop_directory"] = filepath.Join(dir, "taildrop")
}

// sanitizeStateDirName — тег как ИМЯ КАТАЛОГА. Тег свободен (пробелы, слэши,
// двоеточие тег-префикса подписки), имя каталога — нет: недопустимые для
// пути символы заменяются на `_`, пустой результат — на схему.
func sanitizeStateDirName(tag string) string {
	tag = strings.TrimSpace(tag)
	var b strings.Builder
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	// Точечные имена ("." / "..") каталогом не годятся — они означают
	// текущий и родительский каталог, а не имя.
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return SchemeTailscale
	}
	return out
}
