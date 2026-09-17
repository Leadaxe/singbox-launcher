// File endpoint_schemes.go — единственная точка истины «эта схема живёт в
// endpoints[], а не в outbounds[]» (SPEC 122 §2.1).
//
// Раньше признак endpoint'а был строкой `node.Scheme == "wireguard"` в двух
// местах эмиссии (EmitNodeJSONs и GenerateEndpointJSONBare) — и тип,
// добавленный в таблицу схем импорта, но не в эти строки, молча уезжал в
// outbounds[]. Предикат ниже нужен обоим местам и всем будущим.
//
// Нормативный источник — contract/registry/protocols/<scheme>.json, поле
// `kind` (`endpoint` против `outbound`). Реестр в Go сейчас не читается ни
// одним пакетом (это контрактный документ для сверки двух приложений), и
// заводить ради одного предиката загрузчик JSON на старте значило бы
// поставить эмиссию в зависимость от наличия файла на диске. Поэтому здесь
// константная таблица, а её расхождение с реестром ловится глазами при
// правке реестра — как и таблица singboxSchemeByType рядом.
//
// masque сюда НАМЕРЕННО не входит: в sing-box >= 1.11 он тоже endpoint, но
// лаунчер эмитит его в outbounds[] (outbound_generator.go:528), и перенос —
// отдельное решение, а не побочный эффект этой задачи.
package config

import (
	"path/filepath"
	"strings"
	"sync"

	"singbox-launcher/core/config/configtypes"
)

// endpointSchemes — схемы, чьи узлы эмитятся в секцию endpoints[].
// Ключи совпадают с полем `scheme` реестра протоколов.
var endpointSchemes = map[string]bool{
	"wireguard": true, // contract/registry/protocols/wireguard.json  (kind: endpoint)
	"tailscale": true, // contract/registry/protocols/tailscale.json  (kind: endpoint)
}

// IsEndpointScheme — уедет ли узел этой схемы в endpoints[].
func IsEndpointScheme(scheme string) bool {
	return endpointSchemes[scheme]
}

// SchemeTailscale — схема узла tailnet (contract/registry/protocols/tailscale.json).
// Алиас константы модели: предикат IsExitCapable живёт в configtypes
// (leaf-пакет, импортировать config он не может), и строка обязана быть одна.
const SchemeTailscale = configtypes.SchemeTailscale

// tailscaleStateDirRoot — каталог, ВНУТРИ которого лежат каталоги состояния
// узлов tailnet. Ставится приложением (core.AppController) тем же приёмом,
// что и хуки проб: пакет config не знает ни про ExecDir, ни про файловый
// сервис, а эмиссия одна и та же для сборки и для превью вкладки JSON.
//
// Пусто = состояние ставить некуда (превью, тесты): поле не подставляется
// вовсе, и ядро возьмёт свой дефолт. Молча писать относительный путь нельзя
// — рабочий каталог ядра лаунчеру не принадлежит.
// tailscaleRemoteStateDirRoot — корень каталогов состояния tailnet НА
// МАШИНЕ-ИСПОЛНИТЕЛЕ, когда конфиг собирается не для себя
// (`<state_dir>/tailscale` её демона, SPEC 063/122).
//
// Живёт рядом с локальным корнем и переключается вместе с целью Мастера
// (ShowConfigWizardForMachine ставит, Local — снимает), потому что эмиссия
// одна и та же для сборки и для превью вкладки JSON: разъедься они, вкладка
// показывала бы путь, отличный от уехавшего в конфиг.
//
// Пусто = собираем для себя, берётся локальный корень. GC каталогов на это
// НЕ смотрит: он ходит по нашей файловой системе и остаётся на локальном
// корне всегда — чужой каталог нам не принадлежит.
var (
	tailscaleStateDirRoot       string
	tailscaleRemoteStateDirRoot string
	tailscaleStateDirRootMu     sync.RWMutex
)

// SetTailscaleStateDirRoot задаёт корень каталогов состояния tailnet.
// Ожидается абсолютный путь вида `<execDir>/bin/tailscale` — тот же корень
// `<execDir>/bin`, относительно которого эмитятся локальные .srs
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

// SetTailscaleRemoteStateDirRoot задаёт корень состояния tailnet на
// машине-исполнителе. Пустая строка (Local) снимает переопределение.
//
// Зовётся при выборе цели Мастера — там же, где ставится ResourceDir: оба
// поля решают одну задачу, путь чужой машины в конфиг для чужой машины.
func SetTailscaleRemoteStateDirRoot(root string) {
	root = strings.TrimSpace(root)
	tailscaleStateDirRootMu.Lock()
	tailscaleRemoteStateDirRoot = root
	tailscaleStateDirRootMu.Unlock()
}

// TailscaleRemoteStateDirRoot — текущий корень машины-исполнителя.
func TailscaleRemoteStateDirRoot() string {
	tailscaleStateDirRootMu.RLock()
	defer tailscaleStateDirRootMu.RUnlock()
	return tailscaleRemoteStateDirRoot
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
// демона). Непустой = конфиг собирается для чужой машины, и локальный корень
// в него писать нельзя: путь резолвит ядро на той стороне, нашего пути там
// нет. Ядро создало бы его от своего корня, и состояние узла оседало бы в
// каталоге вида `/Applications/…/bin/tailscale/<тег>` на роутере — рабочем,
// но абсурдном и сносимом первой же чисткой overlay.
func applyTailscaleStateDirectory(endpoint map[string]interface{}, scheme, tag, remoteRoot string) {
	if scheme != SchemeTailscale || endpoint == nil {
		return
	}
	if _, present := endpoint["state_directory"]; present {
		return
	}
	if remoteRoot = strings.TrimSpace(remoteRoot); remoteRoot != "" {
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
