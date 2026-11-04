// File tailscale_state_dir.go — ЖИЗНЕННЫЙ ЦИКЛ каталога состояния tailnet
// (SPEC 122, дополнение «Каталог состояния: жизненный цикл»; норма
// согласована с LxBox 14.09.2026).
//
// # Зачем это вообще нужно
//
// `state_directory` узла tailscale — не кэш. В нём лежит `tailscaled.state`
// с ключом устройства: именно он делает узел ТЕМ ЖЕ устройством в tailnet
// между запусками. Одноразовый ключ (`tskey-auth-…`) сгорает на первом
// логине, и потеря каталога значит «зарегистрируй устройство заново» —
// вручную, новым ключом.
//
// Отсюда две обязанности, которых у эмиссии нет:
//
//   - узел ПЕРЕИМЕНОВАЛИ (или перенесли в папку с другой тег-политикой): каталог
//     остаётся на месте, имя держит параметр записи `state_dir_id` (SPEC 179);
//   - узел УДАЛИЛИ — каталог обязан уйти следом, иначе в корне копится кладбище
//     состояний мёртвых узлов.
//
// GC на сборке — страховка от третьего случая: путей, которыми узел исчезает
// или меняет имя, больше одного (Save окна, импорт бэкапа, fetch подписки,
// ручная правка state.json), и вешать вызов на каждый значило бы ловить их
// вечно. Поэтому каждая сборка сверяет содержимое корня с ХРАНИМЫМ составом
// узлов и сносит лишнее.
//
// # Чего здесь НЕТ
//
// Каталога с явным `state_directory` в теле узла: его задал пользователь,
// он лежит вне нашего корня (applyTailscaleStateDirectory такое тело не
// трогает), и ни переименование, ни GC его не касаются.
//
// В бэкап каталог не едет и ехать не может: путь этой машины в теле узла не
// живёт (SPEC 122 §2.1 — `state_directory` штампует только config-форма
// GenerateEndpointJSON, но не GenerateEndpointJSONBare, которым пишется
// канон и экспорт).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

// TailscaleStateDirName — имя каталога состояния по ФИНАЛЬНОМУ тегу узла.
//
// Единственная точка, где тег превращается в имя каталога и для эмиссии, и
// для уборки: разойдись они на один символ — GC снёс бы живое состояние.
func TailscaleStateDirName(finalTag string) string {
	return sanitizeStateDirName(finalTag)
}

// TailscaleStateDirNameFor — имя каталога узла: параметр записи `state_dir_id`,
// а без него — по финальному тегу. Значение приходит и из бэкапа, поэтому
// всегда проходит очистку: путь за пределы корня из него не собрать.
func TailscaleStateDirNameFor(stateDirID, finalTag string) string {
	if id := strings.TrimSpace(stateDirID); id != "" {
		return sanitizeStateDirName(id)
	}
	return TailscaleStateDirName(finalTag)
}

// SeedTailscaleStateDir ставит новому узлу tailnet случайное имя каталога.
// Явный `state_directory` в теле и уже стоящее имя не трогает.
func SeedTailscaleStateDir(n *state.Node) {
	if n == nil || n.StateDirID != "" || n.Kind != state.SourceKindServer || len(n.Body) == 0 {
		return
	}
	if !bodyIsTailscale(n.Body) || tailscaleRawBodyHasExplicitStateDir(n.Body) {
		return
	}
	n.StateDirID = state.NewStateDirID()
}

// tailscaleStateDirPath — абсолютный путь каталога состояния ВНУТРИ корня.
//
// Возвращает "" когда корня нет (превью, тесты) или когда имя после очистки
// выводит за корень. Вторая проверка — не паранойя: имя приходит из тега,
// то есть из подписки, и `os.RemoveAll` по неочищенному пути был бы
// удалением произвольного каталога машины по строке из чужого JSON.
func tailscaleStateDirPath(name string) string {
	root := TailscaleStateDirRoot()
	if root == "" {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Имя обязано быть ОДНИМ сегментом: sanitizeStateDirName заменяет
	// разделители на "_", но путь строится и из имён, пришедших не от него
	// (содержимое каталога при GC), и лишний сегмент здесь недопустим.
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, os.PathSeparator) {
		return ""
	}
	root = filepath.Clean(root)
	p := filepath.Clean(filepath.Join(root, name))
	// После Clean путь обязан лежать строго ВНУТРИ корня: равенство корню
	// значит, что имя схлопнулось в "." и удалялся бы сам корень.
	if p == root || !strings.HasPrefix(p, root+string(os.PathSeparator)) {
		return ""
	}
	return p
}

// RemoveTailscaleStateDir сносит каталог состояния узла (норма 1).
//
// Ошибка удаления — WarnLog и ничего больше: операция пользователя (удаление
// узла) уже произошла в модели, и откатывать её из-за занятого файла значило
// бы показать «узел не удалился» там, где он удалился.
func RemoveTailscaleStateDir(name string) {
	p := tailscaleStateDirPath(name)
	if p == "" {
		return
	}
	if _, err := os.Stat(p); err != nil {
		// Нет каталога — нечего сносить: узел ни разу не логинился, либо
		// каталог уже убрал GC. Это норма, не ошибка.
		return
	}
	if err := os.RemoveAll(p); err != nil {
		debuglog.WarnLog("tailscale state dir: failed to remove %q: %v", p, err)
		return
	}
	debuglog.InfoLog("tailscale state dir: removed %q (node deleted)", p)
}

// GCTailscaleStateDirs сносит каталоги под корнем, которых нет в ОЖИДАЕМОМ
// наборе (норма 3).
//
// `expected` — имена (после sanitize) по ВСЕМ ХРАНИМЫМ узлам tailscale,
// включая выключенные и снятые гейтом ядра: узел, выключенный на месяц, свою
// идентичность терять не обязан. Строит набор вызывающая сторона —
// CollectTailscaleStateDirNames.
//
// `ok=false` = набор построить НЕ УДАЛОСЬ (состояние не читается, канона
// нет). Тогда GC пропускается целиком: снести по пустому набору значит
// снести ВСЁ, а «данных нет» и «узлов нет» — разные вещи (ловушка ленивого
// кэша).
func GCTailscaleStateDirs(expected map[string]bool, ok bool) {
	root := TailscaleStateDirRoot()
	if root == "" {
		return
	}
	if !ok {
		debuglog.WarnLog("tailscale state dir: expected set is unknown — GC skipped (nothing removed)")
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			debuglog.WarnLog("tailscale state dir: cannot read root %q: %v — GC skipped", root, err)
		}
		// Корня нет = ни один узел ещё не поднимался. Это не ошибка.
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			// Файлы в корне не наши: каталоги состояния — каталоги. Чужое не
			// трогаем.
			continue
		}
		name := e.Name()
		if expected[name] {
			continue
		}
		p := tailscaleStateDirPath(name)
		if p == "" {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			debuglog.WarnLog("tailscale state dir: GC failed to remove %q: %v", p, err)
			continue
		}
		debuglog.InfoLog("tailscale state dir: GC removed orphan %q (no such tailscale node)", p)
	}
}

// CollectTailscaleStateDirNames — ОЖИДАЕМЫЙ набор имён каталогов по всем
// хранимым узлам tailscale сборки.
//
// Два источника, и оба обязательны:
//
//   - `emitted` — финальные теги узлов, ДОШЕДШИХ до эмиссии. Только здесь
//     известен суффикс уникализации (`ts-2` при коллизии): его считает
//     глобальный счётчик сборки, а не тег-политика;
//   - канон источников — узлы, до эмиссии НЕ дошедшие: выключенные (их
//     выбрасывает EmitCanonicalSource после тег-машины) и снятые гейтом
//     ядра. Их финальное имя считается политикой контейнера БЕЗ суффикса
//     уникализации — знать его неоткуда, а брать «без суффикса» безопасно:
//     лишнее имя в наборе лишь сохранит каталог, тогда как недостающее его
//     снесло бы.
//
// `ok=false` = собирать не из чего (nil parserConfig): вызывающая сторона
// обязана пропустить GC, а не снести всё.
func CollectTailscaleStateDirNames(parserConfig *ParserConfig, emitted []*ParsedNode) (map[string]bool, bool) {
	if parserConfig == nil {
		return nil, false
	}
	names := make(map[string]bool)

	// 1. Дошедшие до эмиссии — финальный тег как он лёг в конфиг.
	for _, n := range emitted {
		if n == nil || n.Scheme != SchemeTailscale {
			continue
		}
		if tailscaleBodyHasExplicitStateDir(n.Outbound) {
			// Явный state_directory пользователя лежит вне корня: имени в
			// наборе ему не нужно, каталога под корнем у него нет.
			continue
		}
		names[TailscaleStateDirNameFor(n.StateDirID, n.Tag)] = true
	}

	// 2. Все ХРАНИМЫЕ узлы канона — включая выключенные и снятые гейтом.
	for _, ps := range parserConfig.ParserConfig.Proxies {
		cs := ps.Canonical
		if cs == nil {
			continue
		}
		for i := range cs.Nodes {
			cn := &cs.Nodes[i]
			if cn.Kind != canonicalKindServer || len(cn.Body) == 0 {
				continue
			}
			if !bodyIsTailscale(cn.Body) {
				continue
			}
			if tailscaleRawBodyHasExplicitStateDir(cn.Body) {
				continue
			}
			names[TailscaleStateDirNameFor(cn.StateDirID, canonicalStateDirTag(cs, cn))] = true
		}
	}
	return names, true
}

// canonicalStateDirTag — финальный тег узла БЕЗ суффикса уникализации:
// префикс контейнера + сырой тег + постфикс.
//
// Переменные тег-политики ({$num} и прочие) здесь НЕ раскрываются: их
// значение зависит от порядка эмиссии, которого у не дошедшего до эмиссии
// узла нет. Политика с переменной даст имя, отличное от эмиссионного, — и
// это осознанный перекос В СТОРОНУ СОХРАНЕНИЯ: лишнее имя в наборе оставляет
// каталог жить, недостающее — сносит.
func canonicalStateDirTag(cs *configtypes.CanonicalSource, cn *configtypes.CanonicalNode) string {
	tag := cn.Tag
	if !cs.IsContainer {
		return tag
	}
	policy := state.TagPolicy{Prefix: cs.TagPrefix, Postfix: cs.TagPostfix}
	return policy.FinalTag(tag)
}

// bodyIsTailscale — тело узла это endpoint tailnet.
func bodyIsTailscale(body []byte) bool {
	obj, err := decodeOrderedJSONObject(body)
	if err != nil {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(obj.stringValue("type")))
	return canonicalSchemeFromType(t) == SchemeTailscale
}

// tailscaleRawBodyHasExplicitStateDir — в СЫРОМ теле уже стоит
// `state_directory` (пользователь задал сам).
func tailscaleRawBodyHasExplicitStateDir(body []byte) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	return tailscaleBodyHasExplicitStateDir(m)
}

// tailscaleBodyHasExplicitStateDir — в разобранном теле уже стоит
// `state_directory`.
func tailscaleBodyHasExplicitStateDir(body map[string]interface{}) bool {
	if body == nil {
		return false
	}
	// Проверка по НАЛИЧИЮ ключа, а не по его значению — ровно как в
	// applyTailscaleStateDirectory: разойдись они, и узел с пустым
	// `state_directory` получил бы каталог под корнем, которого GC не ждёт.
	_, present := body["state_directory"]
	return present
}
