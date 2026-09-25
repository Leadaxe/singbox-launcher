package subscription

import (
	"fmt"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// NodeIdentityFunc — package-level hook, дающий парсеру доступ к вычислению
// идентичности узла (SPEC 112: идентичность = тег в рамках источника).
//
// Сама функция тривиальна и живёт в пакете config рядом с прочей работой с
// узлами; хук оставлен ради симметрии с LegacyNodeIdentityHashFunc и чтобы
// вызывающие слои не расходились в трактовке узлов-групп.
//
// nil → StampNodeIdentity падает на встроенное правило (тег как есть). Это не
// ошибка: парсер обязан оставаться работоспособным в изоляции — в тестах
// пакета subscription хук не установлен.
var NodeIdentityFunc func(node *configtypes.ParsedNode) string

// LegacyNodeIdentityHashFunc — хук на УПРАЗДНЁННЫЙ контент-хеш
// (config.LegacyNodeIdentityHash), нужный ТОЛЬКО для миграции состояний,
// записанных до SPEC 112.
//
// Хеш считается от ЭМИТИРОВАННОГО outbound-JSON, а эмиттер живёт в пакете
// config, который сам импортирует subscription. Прямой вызов дал бы цикл
// импорта, поэтому зависимость подставляется сверху хуком.
//
// nil → миграция не выполняется: legacy-ключи доживают до следующего запуска,
// а не выбрасываются молча.
var LegacyNodeIdentityHashFunc func(node *configtypes.ParsedNode) string

// StampNodeIdentity снимает идентичность узла (SPEC 112) — сырой тег,
// уникализированный в пределах источника.
//
// Зовётся строго ДО применения tag_prefix / tag_postfix / tag_mask: смена
// политики тегов источника идентичность менять не должна, иначе пользователь
// терял бы отметки выключения при каждой правке префикса.
//
// idCounts — счётчик уникализации ОДНОГО источника (не общий tagCounts
// конфига): идентичность уникальна в рамках источника, а не глобально.
// Алгоритм тот же, что у конфиговых тегов: первый `X`, следующий `X-2`.
//
// Узлы-группы идентичности не получают: цепляться через selector — задача
// DetourTag (SPEC 077), а отметок выключения у групп нет.
func StampNodeIdentity(node *configtypes.ParsedNode, idCounts map[string]int) string {
	if node == nil || node.Scheme == configtypes.SchemeGroup {
		return ""
	}
	raw := strings.TrimSpace(node.Tag)
	if raw == "" {
		return ""
	}
	if idCounts == nil {
		node.IdentityTag = raw
		return raw
	}
	// makeIdentityUnique, а не MakeTagUnique: у той же логики здесь другой
	// журнал — «дубль тега» в списке узлов конфига и «два узла с одним именем
	// у провайдера» это разные события, и WarnLog про второе только шумит.
	node.IdentityTag = makeIdentityUnique(raw, idCounts)
	return node.IdentityTag
}

// makeIdentityUnique повторяет схему MakeTagUnique (`X`, `X-2`, `X-3`) без
// журналирования: дубли имён у провайдера — норма, а не предупреждение.
func makeIdentityUnique(raw string, idCounts map[string]int) string {
	return uniquifyAgainstCounts(raw, idCounts)
}

// MakeIdentityUnique — та же машина уникализации сырых тегов, что у принятых
// узлов (StampNodeIdentity), но без узла на руках.
//
// Экспортирована для материализации неразобранных записей (SPEC 116 W13):
// теперь имя есть и у них (подпись баннера, `tag`/`ps`/`remarks` элемента
// JSON), а значит и столкнуться оно может — и разводить столкновение обязана
// ТА ЖЕ машина, что у соседей по контейнеру. Своя вторая («тег занят →
// позиционный `unsupported-N`») дала бы двум одинаковым баннерам подряд разные
// правила именования, и второй перестал бы матчиться при следующем fetch.
func MakeIdentityUnique(raw string, idCounts map[string]int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || idCounts == nil {
		return raw
	}
	return makeIdentityUnique(raw, idCounts)
}

// uniquifyAgainstCounts подбирает свободное имя вида `X`, `X-2`, `X-3` и
// занимает его в счётчике.
//
// Кандидат ПРОВЕРЯЕТСЯ на занятость (SPEC 113-A §5, находка аудита M2):
// сгенерированное `X-2` может уже принадлежать настоящему имени из подписки.
// Подписка `X, X-2, X` без этой проверки давала `X, X-2, X-2` — две
// идентичности с одним ключом (отметка выключения гасила оба узла), а в
// конфиговых тегах ядро отвергает весь outbounds на дубле тега.
func uniquifyAgainstCounts(name string, counts map[string]int) string {
	if counts[name] == 0 {
		counts[name] = 1
		return name
	}
	for {
		counts[name]++
		candidate := fmt.Sprintf("%s-%d", name, counts[name])
		if counts[candidate] == 0 {
			counts[candidate] = 1
			return candidate
		}
	}
}

// SPEC 112 снёс dedupNodesByIdentity вместе с контент-хешем — и вместе с ним
// уехал дедуп байтовых копий (регресс v1.5.2: подписка из 39 записей, где 32
// одинаковых ss:// различались только `#fragment`, показывала 32 узла вместо
// одного). SPEC 112-B вернул дедуп КАК PARSE-СЛОЙ: ключ — не идентичность, а
// подпись содержимого (dedupSignature, server_conn_key.go); он живёт один разбор
// источника, в состояние не пишется и на отметки/ссылки не влияет.
// Идентичность узла по-прежнему тег, и «тот же сервер под двумя ИМЕНАМИ, но
// с разными кредами» — по-прежнему два узла.

// NormalizeSubscriptionTextLine trims whitespace, drops invalid UTF-8 byte sequences, and replaces
// HTML-escaped "&amp;" with "&". Some public lists are HTML-exported; without this, query parameters
// stay merged and URI parsing breaks.
func NormalizeSubscriptionTextLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}

// IsSubscriptionURL checks if the input string is a subscription URL (http:// or https://)
func IsSubscriptionURL(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "http://") ||
		strings.HasPrefix(trimmed, "https://")
}

// MakeTagUnique makes a tag unique by appending a number if it already exists in tagCounts.
// Updates tagCounts map and returns the unique tag.
// logPrefix is used for logging (e.g., "Parser" or "ConfigWizard").
//
// Сгенерированный суффикс проверяется на занятость (SPEC 113-A §5): подписка
// `X, X-2, X` раньше давала два тега `X-2`, и ядро отвергало весь массив
// outbounds — дубль тега для sing-box фатален.
func MakeTagUnique(tag string, tagCounts map[string]int, logPrefix string) string {
	if tagCounts[tag] == 0 {
		// First occurrence of this tag
		tagCounts[tag] = 1
		return tag
	}
	occurrence := tagCounts[tag] + 1
	uniqueTag := uniquifyAgainstCounts(tag, tagCounts)
	debuglog.WarnLog("%s: Duplicate tag '%s' found (occurrence #%d), renamed to '%s'", logPrefix, tag, occurrence, uniqueTag)
	return uniqueTag
}

// LogDuplicateTagStatistics logs statistics about duplicate tags found during processing
func LogDuplicateTagStatistics(tagCounts map[string]int, logPrefix string) {
	duplicatesFound := false
	for tag, count := range tagCounts {
		if count > 1 {
			if !duplicatesFound {
				debuglog.DebugLog("%s: === Duplicate Tag Statistics ===", logPrefix)
				duplicatesFound = true
			}
			debuglog.WarnLog("%s: Tag '%s' appeared %d times (original + %d duplicates)", logPrefix, tag, count, count-1)
		}
	}
	if duplicatesFound {
		debuglog.DebugLog("%s: === End of Duplicate Tag Statistics ===", logPrefix)
	}
}

// applyTagPrefixPostfix applies prefix and postfix to a node tag if specified in ProxySource.
// Supports variable substitution in prefix and postfix.
// Returns the modified tag.
//
// SPEC 118 W5: маски тегов больше нет — в каноне v7 тег узла хранится полем
// (Node.Tag), а тег-политика контейнера это ровно префикс с постфиксом.
func applyTagPrefixPostfix(node *configtypes.ParsedNode, tagPrefix, tagPostfix string, nodeNum int) string {
	tag := node.Tag

	// Replace variables in prefix
	if tagPrefix != "" {
		prefix := replaceTagVariables(tagPrefix, node, nodeNum)
		tag = prefix + tag
	}

	// Replace variables in postfix
	if tagPostfix != "" {
		postfix := replaceTagVariables(tagPostfix, node, nodeNum)
		tag = tag + postfix
	}

	return tag
}

// replaceTagVariables replaces variables in tag prefix/postfix with actual values from node.
// Supported variables:
//   - {$tag} - original node tag
//   - {$scheme} or {$protocol} - protocol (vless, vmess, trojan, ss, hysteria2)
//   - {$server} - server address
//   - {$port} - server port (number)
//   - {$label} - label from URL (fragment after #)
//   - {$comment} - comment
//   - {$num} - node sequential number starting from 1
func replaceTagVariables(template string, node *configtypes.ParsedNode, nodeNum int) string {
	result := template

	// Replace {$tag}
	result = strings.ReplaceAll(result, "{$tag}", node.Tag)

	// Replace {$scheme} or {$protocol}
	result = strings.ReplaceAll(result, "{$scheme}", node.Scheme)
	result = strings.ReplaceAll(result, "{$protocol}", node.Scheme)

	// Replace {$server}
	result = strings.ReplaceAll(result, "{$server}", node.Server)

	// Replace {$port}
	result = strings.ReplaceAll(result, "{$port}", strconv.Itoa(node.Port))

	// Replace {$label}
	result = strings.ReplaceAll(result, "{$label}", node.Label)

	// Replace {$comment}
	result = strings.ReplaceAll(result, "{$comment}", node.Comment)

	// Replace {$num}
	result = strings.ReplaceAll(result, "{$num}", strconv.Itoa(nodeNum))

	return result
}
