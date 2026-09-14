package backup

// Файловый слой LX Backup: запись файла формата 1.0 и разбор файлов обоих
// читаемых форматов (0.x и 1.0).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// MaxFileBytes — потолок читаемого файла (8 MiB).
//
// Бэкап — это настройки, а не данные: реальный файл измеряется десятками
// килобайт. Потолок защищает от попытки скормить импортёру произвольный
// большой файл, а не ограничивает пользователя.
const MaxFileBytes = 8 << 20

// extensionsKey — упразднённый механизм провоза (BACKUP_PRINCIPLES.md П3).
// Имя осталось в коде ровно затем, чтобы файл 0.10.x был опознан и объяснён
// пользователю одним внятным warning'ом, а не рассыпался на десяток «ключ
// такой-то не понят».
const extensionsKey = "extensions"

// WriteFile10 сохраняет бэкап формата 1.0 — единственного, который лаунчер
// пишет (D-110).
//
// Пишется с отступами: файл читают и правят руками, а компактный JSON в одну
// строку делает это невозможным. Запись атомарная — прерванная запись не
// должна оставить обрезанный файл вместо прежнего.
func WriteFile10(path string, b *Backup10) error {
	if b == nil {
		return fmt.Errorf("nil backup")
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("backup serialization: %w", err)
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// writeFileAtomic — общая запись через временный файл и rename.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	return nil
}

// FileFormat — какой из читаемых форматов у разобранного файла.
//
// Нужен только ЧТЕНИЮ: писатель у лаунчера один (1.0, D-110), а входов два —
// файлы 0.x уже у пользователей на руках, и читаются они всегда. Нулевое
// значение не означает ни одного формата: его несёт только File, который не
// прошёл через Parse.
type FileFormat int

const (
	// FileFormatLegacy — семейство 0.x (`lx_backup: 1`), вход legacy_read_0x.go.
	FileFormatLegacy FileFormat = iota + 1
	// FileFormat10 — контракт 1.0 (`lx_backup: 2`), вход import10.go.
	FileFormat10
)

// File — разобранный файл бэкапа любого читаемого формата.
//
// Union, а не два входа у каждого вызывающего: UI, debug API и корпус читают
// файл, не зная и не желая знать, каким писателем он сделан. Развилка живёт в
// одном месте — ImportFile (import.go).
type File struct {
	// Format — каким форматом сделан файл (по маркеру `lx_backup`).
	Format FileFormat
	// Legacy — форма 0.x (lx_backup: 1); nil у файла 1.0.
	Legacy *Backup
	// V10 — форма 1.0 (lx_backup: 2); nil у файла 0.x.
	V10 *Backup10
}

// ExportedByOf — шапка файла независимо от формата (нужна сводке импорта).
func (f *File) ExportedByOf() (ExportedBy, string) {
	switch {
	case f == nil:
		return ExportedBy{}, ""
	case f.V10 != nil:
		return f.V10.ExportedBy, f.V10.ExportedAt
	case f.Legacy != nil:
		return f.Legacy.ExportedBy, f.Legacy.ExportedAt
	}
	return ExportedBy{}, ""
}

// Counts — сколько чего лежит в файле: источники, правила, переменные.
//
// Одно число на источники, а не «подписки и серверы» по отдельности: у
// формата 1.0 секция одна (sources[]), и раскладывать её обратно на две ради
// сводки значило бы завести у UI знание о форме файла.
func (f *File) Counts() (sources, rules, vars int) {
	switch {
	case f == nil:
		return 0, 0, 0
	case f.V10 != nil:
		return len(f.V10.Sources), len(f.V10.Rules), len(f.V10.Vars)
	case f.Legacy != nil:
		b := f.Legacy
		return len(b.Subscriptions) + len(b.Servers) + len(b.Chains), len(b.Rules), len(b.Vars)
	}
	return 0, 0, 0
}

// ReadFile читает и разбирает бэкап.
func ReadFile(path string) (*File, []Warning, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("backup file: %w", err)
	}
	if info.Size() > MaxFileBytes {
		return nil, nil, fmt.Errorf("backup file exceeds %d bytes", MaxFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse разбирает содержимое бэкапа и называет всё, что импортёр не применит.
//
// Формат опознаётся по `lx_backup` ДО разбора тела: 1 — 0.x, 2 — контракт 1.0.
// Одно поле, один int, никакой второй строковой версии в корне — два способа
// сказать «какой это формат» разошлись бы на первом же релизе. Значение вне
// этих двух — отказ ПРЕЖНИМ кодом: файл более новой мажорной версии читать
// нечем, и сделать вид, что прочитали, хуже честного отказа.
//
// Схема намеренно открыта (additionalProperties: true) — файл более новой
// минорной версии или чужой стороны обязан читаться. Но применять неизвестное
// молча нельзя (П3): каждый ключ вне модели предъявляется пользователем с
// именем сущности, в которой он встретился.
//
// Разбор терпим к ЧУЖОМУ ТИПУ знакомого ключа. Ключ, известный обеим сторонам
// под разными типами, — не «сломанный файл», а разошедшаяся деталь формата:
// `subscriptions[].skip` у LxBox 0.10.x boolean, у launcher — список фильтров
// отсева. Строгий разбор ронял бы весь импорт из-за одного такого поля, то
// есть терял бы всё остальное молча — ровно то, что запрещает П6. Поэтому
// несовпавшее по типу поле отбрасывается с warning, а импорт продолжается.
func Parse(data []byte) (*File, []Warning, error) {
	var head struct {
		LxBackup int `json:"lx_backup"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		// Тип у lx_backup чужой либо документ битый: терпимый разбор ниже
		// разберётся и скажет точнее — здесь важно лишь не решить формат
		// по мусору.
		head.LxBackup = 0
	}
	switch head.LxBackup {
	case 0:
		return nil, nil, fmt.Errorf("not an LX Backup file: lx_backup field missing")
	case FormatVersion:
		b, typeWarns, err := decodeTolerant(data)
		if err != nil {
			return nil, nil, fmt.Errorf("backup parse: %w", err)
		}
		return &File{Format: FileFormatLegacy, Legacy: b},
			append(typeWarns, scanUnknown(data)...), nil
	case FormatVersion10:
		b, typeWarns, err := decodeTolerant10(data)
		if err != nil {
			return nil, nil, fmt.Errorf("backup parse: %w", err)
		}
		return &File{Format: FileFormat10, V10: b},
			append(typeWarns, scanUnknown10(data)...), nil
	default:
		return nil, nil, fmt.Errorf("backup format v%d is newer than supported v%d — update the app",
			head.LxBackup, FormatVersion10)
	}
}

// maxTypeMismatchPasses — потолок проходов терпимого разбора.
//
// Каждый проход снимает ОДИН путь-ключ, поэтому число проходов ограничено
// числом полей модели; потолок здесь не логика, а страховка от бесконечного
// цикла, если бы декодер вдруг сообщал путь, который нечего вырезать.
const maxTypeMismatchPasses = 64

// decodeTolerant разбирает файл, вырезая поля, тип которых не совпал с моделью.
//
// Механика опирается на свойство encoding/json: при несовпадении типа декодер
// НЕ бросает разбор — он пропускает значение, дочитывает остальной документ и
// возвращает первую такую ошибку в конце. Значит достаточно узнать путь поля
// из *json.UnmarshalTypeError, вырезать его из сырого дерева и повторить: за
// несколько проходов собираются все несовпадения, и каждое становится
// отдельным warning'ом с полным путём записи.
//
// Ошибка НЕ типа (битый JSON, обрезанный файл) остаётся фатальной: там нечего
// спасать, и делать вид, что файл прочитан, было бы хуже отказа.
func decodeTolerant(data []byte) (*Backup, []Warning, error) {
	var out Backup
	warns, err := decodeTolerantInto(data, &out, legacyArrayLabelKeys)
	if err != nil {
		return nil, nil, err
	}
	return &out, warns, nil
}

// decodeTolerant10 — то же для формата 1.0.
//
// Отдельная обёртка, а не дженерик на две структуры: разница между ними ровно
// в типе цели и в таблице имён записей, а обобщение через reflect стоило бы
// читаемости там, где читаемость и есть весь смысл этого кода.
func decodeTolerant10(data []byte) (*Backup10, []Warning, error) {
	var out Backup10
	warns, err := decodeTolerantInto(data, &out, arrayLabelKeys10)
	if err != nil {
		return nil, nil, err
	}
	return &out, warns, nil
}

// decodeTolerantInto — общий механизм терпимого разбора.
func decodeTolerantInto(data []byte, target interface{}, labels map[string]string) ([]Warning, error) {
	var warns []Warning
	cur := data

	for pass := 0; pass < maxTypeMismatchPasses; pass++ {
		err := json.Unmarshal(cur, target)
		if err == nil {
			return warns, nil
		}
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) || typeErr.Field == "" {
			return nil, err
		}
		next, places := stripField(cur, typeErr.Field, labels)
		if next == nil {
			// Путь не нашёлся — вырезать нечего, и следующий проход дал бы
			// ту же ошибку. Отдаём исходную ошибку, а не крутим цикл.
			return nil, err
		}
		for _, place := range places {
			warns = append(warns, Warning{Code: WarnBackupFieldTypeMismatch, Detail: place})
		}
		cur = next
	}
	return nil, fmt.Errorf("too many type mismatches to recover")
}

// stripField удаляет ключ по ПУТИ вида "subscriptions.skip" из сырого дерева.
//
// Путь из UnmarshalTypeError не содержит индексов элементов, поэтому ключ
// снимается у ВСЕХ записей секции. Это не огрубление: тип поля у записи один
// на весь файл — сторона, писавшая его boolean'ом, писала так везде. Зато
// имена затронутых записей возвращаются полностью, и пользователь видит не
// «поле skip», а «subscriptions[https://…].skip».
//
// Возвращает nil, если по пути ничего не нашлось.
func stripField(data []byte, path string, labels map[string]string) ([]byte, []string) {
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return nil, nil
	}
	parts := strings.Split(path, ".")
	places := stripPath(root, "", parts, labels)
	if len(places) == 0 {
		return nil, nil
	}
	out, err := json.Marshal(root)
	if err != nil {
		return nil, nil
	}
	sort.Strings(places)
	return out, places
}

// stripPath спускается по пути внутри объекта, проходя массивы насквозь.
func stripPath(obj map[string]json.RawMessage, where string, parts []string, labels map[string]string) []string {
	if len(parts) == 0 {
		return nil
	}
	key := parts[0]
	raw, ok := obj[key]
	if !ok {
		return nil
	}

	if len(parts) == 1 {
		delete(obj, key)
		return []string{joinPath(where, key)}
	}

	// Секция-список: путь общий для всех её записей.
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) == nil {
		var places []string
		changed := false
		for i, item := range items {
			label := joinPath(where, key) + "[" + entryLabel(item, labels[key], i) + "]"
			found := stripPath(item, label, parts[1:], labels)
			if len(found) > 0 {
				changed = true
				places = append(places, found...)
			}
		}
		if !changed {
			return nil
		}
		if repacked, err := json.Marshal(items); err == nil {
			obj[key] = repacked
		}
		return places
	}

	var nested map[string]json.RawMessage
	if json.Unmarshal(raw, &nested) != nil {
		return nil
	}
	places := stripPath(nested, joinPath(where, key), parts[1:], labels)
	if len(places) == 0 {
		return nil
	}
	if repacked, err := json.Marshal(nested); err == nil {
		obj[key] = repacked
	}
	return places
}

// legacyArrayLabelKeys — чем называется запись секции 0.x пользователю. Тот
// же выбор, что у scanUnknown: «в подписке https://…», а не «в записи №3».
var legacyArrayLabelKeys = map[string]string{
	"subscriptions": "url",
	"servers":       "label",
	"chains":        "tag",
	"directions":    "tag",
	"rules":         "name",
	"outbounds":     "tag",
}

// arrayLabelKeys10 — то же для 1.0, где секция источников одна (sources[]).
//
// Запись источника называется тегом: у узла и цепочки он и есть имя, у
// подписки и папки имя лежит в `name`, поэтому у безымянного контейнера
// сработает откат на «#N» (entryLabel). Второе имя вводить незачем: таблица
// нужна ради узнаваемости строки предупреждения, а не ради полноты.
var arrayLabelKeys10 = map[string]string{
	"sources":    "tag",
	"nodes":      "tag",
	"directions": "tag",
	"rules":      "name",
}

func joinPath(where, key string) string {
	if where == "" {
		return key
	}
	return where + "." + key
}

// entitySchema — множество ключей, которые модель понимает у одной сущности.
//
// Список ведётся здесь, а не выводится рефлексией из структур, потому что он
// нормативен: это ровно таблица полей BACKUP.md §2. Рефлексия сделала бы
// «схемой» текущую форму Go-структур, и любое внутреннее переименование молча
// меняло бы контракт.
var (
	rootKeys = map[string]bool{
		"lx_backup": true, "exported_by": true, "exported_at": true,
		"subscriptions": true, "servers": true, "directions": true,
		"chains": true, "rules": true, "dns": true, "vars": true,
		"route": true, "warp": true,
	}
	exportedByKeys = map[string]bool{"app": true, "version": true, "platform": true}
	sourceRefKeys  = map[string]bool{
		"detour_tag": true, "detour_node_source_id": true,
		"detour_node_tag": true, "detour_node_label": true,
	}
	// identity объявлен ЗДЕСЬ, но внутрь него общий обход не спускается:
	// неприменённые ключи объекта считает сам импорт (importSourceIdentity)
	// и выдаёт ОДИН backup_source_identity_dropped на подписку с перечнем.
	// Спустись сюда scanUnknown — device_os дал бы вдобавок
	// backup_unknown_field, то есть два предупреждения об одной потере.
	subscriptionKeys = mergeKeys(sourceRefKeys, map[string]bool{
		"id": true, "url": true, "label": true, "enabled": true,
		"max_nodes": true, "tag": true, "update": true, "disabled": true,
		"skip": true, "outbounds": true, "fold": true, "identity": true,
		"exclude_from_global": true, "expose_group_tags_to_global": true,
	})
	serverKeys = mergeKeys(sourceRefKeys, map[string]bool{
		"id": true, "uri": true, "config_json": true, "label": true,
		"node_tag": true, "enabled": true, "exclude_from_global": true,
		"folder": true, "sections": true,
	})
	// serverSectionsKeys — секции узла (SPEC 121 §10.5). Внутрь записей
	// сканер не спускается намеренно: их поля — тела sing-box плюс служебные
	// kind/enabled/order_num, и их набор ведёт схема состояния, а не таблица
	// бэкапа.
	serverSectionsKeys = map[string]bool{
		"rules": true, "dns": true,
	}
	chainKeys = mergeKeys(sourceRefKeys, map[string]bool{
		"id": true, "tag": true, "label": true, "enabled": true,
		"chain": true, "exclude_from_global": true,
	})
	// label объявлен в схеме с контракта 0.12.4 (D-094) как поле LxBox:
	// он его пишет и читает, лаунчер игнорирует молча. Неизвестным ключом
	// поле больше не считается — иначе каждый импорт файла LxBox давал бы
	// backup_unknown_field на каждом Направлении.
	// ping_url/ping_timeout_ms — то же с 0.12.6 (D-096): бюджет замера
	// Направления у LxBox, лаунчер игнорирует молча.
	directionKeys = map[string]bool{
		"tag": true, "label": true, "ping_url": true, "ping_timeout_ms": true,
		"enabled": true, "filter": true,
		"invert": true, "default": true, "include_direct": true,
		"include_block": true, "include": true,
		"interrupt_exist_connections": true, "auto": true,
	}
	// refs — kind=srs, все наборы одного правила (черновик D-100): объявлено
	// в схеме, поэтому не «непонятое», а поле, которое импортёр применяет.
	ruleKeys = map[string]bool{
		"kind": true, "name": true, "enabled": true, "num": true,
		"outbound": true, "ref": true, "refs": true, "vars": true, "match": true,
		"dns": true, "resolve": true,
	}
	directionAutoKeys = map[string]bool{
		"mode": true, "url": true, "interval": true, "tolerance": true,
		"idle_timeout": true, "interrupt_exist_connections": true,
		"pool": true, "pool_tolerance": true, "sticky_hash": true,
	}
	// chainBodyKeys — канон source_chain.schema.json. Внутрь хопов сканер не
	// спускается намеренно: хоп ссылается на узел выражениями, чей набор
	// ключей ведёт схема цепочки, а не таблица бэкапа.
	chainBodyKeys = map[string]bool{
		"hops": true, "idle_timeout": true, "interrupt_exist_connections": true,
		"rewrite": true, "strip": true, "strip_evasion": true,
	}
	// warpKeys — поля регистрации WG/MASQUE. Union обоих типов: запись
	// объявляет свой type, и разбирать её по типу значило бы завести две
	// почти одинаковые таблицы ради ключей, которых у чужого типа всё равно
	// не бывает.
	warpKeys = map[string]bool{
		"type": true, "private_key": true, "peer_public": true,
		"client_v4": true, "client_v6": true, "client_id": true,
		"device_id": true, "token": true, "account_id": true,
		"license": true, "warp_plus": true, "created_at": true,
		"private_key_der": true, "server_pub_der": true,
		"server": true, "port": true,
	}
	dnsKeys    = map[string]bool{"servers": true, "rules": true, "final": true, "strategy": true}
	dnsRefKeys = map[string]bool{
		"kind": true, "tag": true, "name": true, "enabled": true,
		"num": true, "ref": true, "vars": true, "value": true,
	}
	routeKeys = map[string]bool{"final": true}
)

func mergeKeys(base, extra map[string]bool) map[string]bool {
	out := make(map[string]bool, len(base)+len(extra))
	for k := range base {
		out[k] = true
	}
	for k := range extra {
		out[k] = true
	}
	return out
}

// scanUnknown обходит файл и перечисляет всё, чего нет в модели.
//
// Два разных класса потерь — два разных сообщения (П6):
//
//   - `extensions` любой глубины: ОДИН warning на файл с перечнем затронутых
//     записей. Это не «лишний ключ», а упразднённый карман с произвольным
//     содержимым, и перечислять его внутренности по одной значило бы утопить
//     пользователя в списке вместо объяснения;
//   - всё прочее: warning с именем ключа и сущности, в которой он встретился.
//
// Обход спускается на ВСЮ глубину модели, а не только по верхним записям
// секций: SPEC §3 обещает «неизвестные ключи любого уровня → warning», и
// вложенный уровень — самое удобное место спрятать чужое поле. Путь в warning
// пишется целиком (subscriptions[https://…].outbounds[vpn-1].key), иначе
// предупреждение не с чем сопоставить.
func scanUnknown(data []byte) []Warning {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}

	sc := &unknownScan{}
	sc.object("", root, rootKeys)
	sc.nested(root, "exported_by", exportedByKeys)
	sc.nested(root, "route", routeKeys)

	if raw, ok := root["dns"]; ok {
		var dns map[string]json.RawMessage
		if json.Unmarshal(raw, &dns) == nil {
			sc.object("dns", dns, dnsKeys)
			sc.array(dns, "dns.servers", "servers", dnsRefKeys, "kind", nil)
			sc.array(dns, "dns.rules", "rules", dnsRefKeys, "kind", nil)
		}
	}

	sc.array(root, "subscriptions", "subscriptions", subscriptionKeys, "url", func(where string, item map[string]json.RawMessage) {
		// Локальные Направления источника — та же каноническая форма, что
		// и directions[] на корне, и проверяются тем же множеством ключей:
		// две таблицы для одной сущности разъехались бы.
		sc.array(item, where+".outbounds", "outbounds", directionKeys, "tag", sc.scanDirectionBody)
	})
	sc.array(root, "servers", "servers", serverKeys, "label", func(where string, item map[string]json.RawMessage) {
		// SPEC 121: секции узла — свой уровень ключей; внутрь фрагментов
		// обход не спускается (это тела sing-box, не поля бэкапа).
		sc.nested2(item, where, "sections", serverSectionsKeys)
	})
	sc.array(root, "chains", "chains", chainKeys, "tag", func(where string, item map[string]json.RawMessage) {
		sc.nested2(item, where, "chain", chainBodyKeys)
	})
	sc.array(root, "directions", "directions", directionKeys, "tag", sc.scanDirectionBody)
	sc.array(root, "rules", "rules", ruleKeys, "name", nil)
	sc.array(root, "warp", "warp", warpKeys, "type", nil)

	return sc.warnings()
}

// scanDirectionBody — вложенные уровни одного Направления. Общий для
// корневых directions[] и локальных subscriptions[].outbounds[]: форма у них
// одна, и проверять её двумя разными обходами значило бы завести расхождение.
func (sc *unknownScan) scanDirectionBody(where string, item map[string]json.RawMessage) {
	sc.nested2(item, where, "auto", directionAutoKeys)
}

// unknownScan копит находки обхода: обычные неизвестные ключи по одному и
// упразднённый extensions — списком затронутых записей.
type unknownScan struct {
	fields        []string
	seenField     map[string]bool
	extensionsAt  []string
	seenExtension map[string]bool
}

func (sc *unknownScan) note(where, key string) {
	if key == extensionsKey {
		if sc.seenExtension == nil {
			sc.seenExtension = map[string]bool{}
		}
		place := where
		if place == "" {
			place = "<file root>"
		}
		if !sc.seenExtension[place] {
			sc.seenExtension[place] = true
			sc.extensionsAt = append(sc.extensionsAt, place)
		}
		return
	}
	name := key
	if where != "" {
		name = where + "." + key
	}
	if sc.seenField == nil {
		sc.seenField = map[string]bool{}
	}
	if sc.seenField[name] {
		return
	}
	sc.seenField[name] = true
	sc.fields = append(sc.fields, name)
}

func (sc *unknownScan) object(where string, obj map[string]json.RawMessage, known map[string]bool) {
	for key := range obj {
		if !known[key] {
			sc.note(where, key)
		}
	}
}

func (sc *unknownScan) nested(parent map[string]json.RawMessage, key string, known map[string]bool) {
	raw, ok := parent[key]
	if !ok {
		return
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return
	}
	sc.object(key, obj, known)
}

// nested2 — вложенный объект внутри записи, с путём от корня. Отдельно от
// nested, потому что там путь равен имени ключа, а здесь запись уже названа
// (subscriptions[https://…]) и имя нужно дописать к ней, а не заменить.
func (sc *unknownScan) nested2(parent map[string]json.RawMessage, where, key string, known map[string]bool) {
	raw, ok := parent[key]
	if !ok {
		return
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return
	}
	sc.object(joinPath(where, key), obj, known)
}

// array обходит секцию-список. labelKey — поле, которым запись называется
// пользователю: код обязан показать «в подписке https://…», а не «в записи
// №3», иначе предупреждение не с чем сопоставить.
//
// deeper, если задан, вызывается на каждой записи с её ПОЛНЫМ путём — им
// секция спускается на свои вложенные уровни.
func (sc *unknownScan) array(parent map[string]json.RawMessage, where, key string, known map[string]bool, labelKey string, deeper func(string, map[string]json.RawMessage)) {
	raw, ok := parent[key]
	if !ok {
		return
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return
	}
	for i, item := range items {
		entry := where + "[" + entryLabel(item, labelKey, i) + "]"
		sc.object(entry, item, known)
		if deeper != nil {
			deeper(entry, item)
		}
	}
}

func entryLabel(item map[string]json.RawMessage, labelKey string, index int) string {
	if raw, ok := item[labelKey]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
	}
	return "#" + strconv.Itoa(index+1)
}

func (sc *unknownScan) warnings() []Warning {
	var out []Warning
	if len(sc.extensionsAt) > 0 {
		places := append([]string(nil), sc.extensionsAt...)
		sort.Strings(places)
		out = append(out, Warning{Code: WarnBackupExtensionsDropped, Detail: strings.Join(places, ", ")})
	}
	fields := append([]string(nil), sc.fields...)
	sort.Strings(fields)
	for _, name := range fields {
		out = append(out, Warning{Code: WarnBackupUnknownField, Detail: name})
	}
	return out
}

// SuggestFileName — имя файла по умолчанию для диалога сохранения.
func SuggestFileName(stamp string) string {
	stamp = strings.TrimSpace(stamp)
	if stamp == "" {
		return "lx-backup.json"
	}
	return "lx-backup-" + stamp + ".json"
}
