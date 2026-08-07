package subscription

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// LookupCachedBody — package-level hook, позволяющий вызывающему слою
// (Update / Rebuild в core) подать pre-fetched body для подписок без
// network call'а.
//
// Контракт:
//   - URL — Source.URL текущей подписки.
//   - Возвращает (decoded body, true) если cache hit; (_, false) — fallback
//     на стандартный FetchSubscription.
//   - SPEC 052 phase 6: Update пишет `bin/subscriptions/<id>.raw`, потом
//     устанавливает hook чтобы парсер не дёргал сеть второй раз. Rebuild
//     ставит hook → читает raw → парсит без сети.
//
// nil → стандартное поведение (FetchSubscription).
var LookupCachedBody func(url string) ([]byte, bool)

// NodeIdentityHashFunc — package-level hook, дающий парсеру доступ к
// вычислению идентичности узла (SPEC 094 D2).
//
// Хеш считается от ЭМИТИРОВАННОГО outbound-JSON, а эмиттер живёт в пакете
// config, который сам импортирует subscription. Прямой вызов дал бы цикл
// импорта, поэтому используется тот же приём, что и для LookupCachedBody:
// зависимость подставляется сверху.
//
// nil → дедупликация не выполняется (узлы отдаются как есть). Это не ошибка:
// парсер обязан оставаться работоспособным в изоляции — в тестах пакета
// subscription хук не установлен.
var NodeIdentityHashFunc func(node *configtypes.ParsedNode) string

// disabledNodeTTL возвращает срок жизни отметки о выключенной ноде
// (SPEC 094 D4).
//
// clamp(3 × интервал обновления, 24h, 30d). Три цикла обновления — запас на
// провайдера, у которого нода временно исчезла из выдачи: если удалять отметку
// сразу, нода вернулась бы включённой за спиной пользователя. Верхняя граница
// не даёт отметкам копиться годами.
func disabledNodeTTL(updateIntervalHours int) time.Duration {
	const (
		minTTL = 24 * time.Hour
		maxTTL = 30 * 24 * time.Hour
	)
	if updateIntervalHours <= 0 {
		return minTTL
	}
	ttl := time.Duration(updateIntervalHours) * 3 * time.Hour
	if ttl < minTTL {
		return minTTL
	}
	if ttl > maxTTL {
		return maxTTL
	}
	return ttl
}

// filterDisabledNodes убирает узлы, выключенные пользователем, и обновляет
// временные метки у переживших отметок (SPEC 094 D4).
//
// Возвращает оставшиеся узлы и карту отметок с обновлёнными временами. Карта
// возвращается новой: вызывающий решает, сохранять ли её (GC выполняется только
// при успешном сетевом обновлении, иначе кэш-прогон без сети стёр бы отметки
// для нод, которых временно нет в теле).
func filterDisabledNodes(
	nodes []*configtypes.ParsedNode,
	disabled map[string]int64,
	now time.Time,
) ([]*configtypes.ParsedNode, map[string]int64) {
	if len(disabled) == 0 || NodeIdentityHashFunc == nil {
		return nodes, disabled
	}

	refreshed := make(map[string]int64, len(disabled))
	for hash, ts := range disabled {
		refreshed[hash] = ts
	}

	kept := make([]*configtypes.ParsedNode, 0, len(nodes))
	dropped := 0
	for _, node := range nodes {
		if node == nil {
			continue
		}
		hash := NodeIdentityHashFunc(node)
		if hash == "" {
			kept = append(kept, node)
			continue
		}
		if _, off := disabled[hash]; off {
			// Нода на месте — отметка актуальна, продлеваем.
			refreshed[hash] = now.Unix()
			dropped++
			continue
		}
		kept = append(kept, node)
	}

	if dropped > 0 {
		debuglog.DebugLog("Parser: %d node(s) skipped as disabled by the user", dropped)
	}
	return kept, refreshed
}

// GCDisabledNodes убирает отметки о выключенных нодах, которых давно нет в
// подписке (SPEC 094 D4). TTL считается из интервала обновления источника.
//
// Вызывать ТОЛЬКО после успешного СЕТЕВОГО обновления: на прогоне из кэша тело
// может быть неполным, и отметки исчезли бы для живых нод — те молча включились
// бы обратно за спиной пользователя.
func GCDisabledNodes(disabled map[string]int64, updateIntervalHours int, now time.Time) map[string]int64 {
	return gcDisabledNodes(disabled, disabledNodeTTL(updateIntervalHours), now)
}

// gcDisabledNodes — внутренняя реализация с явным TTL (удобна для тестов).
func gcDisabledNodes(disabled map[string]int64, ttl time.Duration, now time.Time) map[string]int64 {
	if len(disabled) == 0 {
		return disabled
	}
	cutoff := now.Add(-ttl).Unix()
	kept := make(map[string]int64, len(disabled))
	expired := 0
	for hash, ts := range disabled {
		if ts < cutoff {
			expired++
			continue
		}
		kept[hash] = ts
	}
	if expired > 0 {
		debuglog.DebugLog("Parser: dropped %d expired disabled-node mark(s)", expired)
	}
	return kept
}

// dedupNodesByIdentity схлопывает узлы с совпадающей идентичностью в пределах
// ОДНОГО источника (SPEC 094 D3). Выживает первый по порядку.
//
// Между источниками дедуп не применяется: подписаться на две подписки с общим
// сервером — осознанный выбор пользователя, и молча выкидывать одну из них
// значило бы решать за него.
//
// Узлы без вычислимой идентичности (хук не установлен, эмиссия не удалась)
// пропускаются нетронутыми: схлопывать их в одну «пустую» группу нельзя.
func dedupNodesByIdentity(nodes []*configtypes.ParsedNode) []*configtypes.ParsedNode {
	if NodeIdentityHashFunc == nil || len(nodes) < 2 {
		return nodes
	}

	seen := make(map[string]string, len(nodes))
	kept := make([]*configtypes.ParsedNode, 0, len(nodes))
	dropped := 0

	for _, node := range nodes {
		if node == nil {
			continue
		}
		hash := NodeIdentityHashFunc(node)
		if hash == "" {
			kept = append(kept, node)
			continue
		}
		if firstTag, dup := seen[hash]; dup {
			debuglog.DebugLog("Parser: dedup: %q duplicates %q (same identity) — dropped", node.Tag, firstTag)
			dropped++
			continue
		}
		seen[hash] = node.Tag
		kept = append(kept, node)
	}

	if dropped > 0 {
		debuglog.DebugLog("Parser: dedup: dropped %d duplicate node(s) within the source", dropped)
	}
	return kept
}

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
func MakeTagUnique(tag string, tagCounts map[string]int, logPrefix string) string {
	if tagCounts[tag] > 0 {
		// Tag already exists, make it unique
		tagCounts[tag]++
		uniqueTag := fmt.Sprintf("%s-%d", tag, tagCounts[tag])
		debuglog.WarnLog("%s: Duplicate tag '%s' found (occurrence #%d), renamed to '%s'", logPrefix, tag, tagCounts[tag], uniqueTag)
		return uniqueTag
	}

	// First occurrence of this tag
	tagCounts[tag] = 1
	return tag
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

// SourceLoadResult — расширенный результат разбора источника (SPEC 094 A5).
//
// Помимо узлов несёт то, что появляется только при импорте sing-box JSON:
// группы selector/urltest и перечень непрочитанных секций конфига.
type SourceLoadResult struct {
	// Nodes — узлы с уже применёнными тегами (префикс/маска/уникализация).
	//
	// Импортированные из конфига группы (SchemeGroup) лежат здесь же, среди
	// обычных узлов: для лаунчера это рядовые ноды без привилегий, а не записи
	// вкладки Outbounds, на которые ссылается роутинг (SPEC 094 A5).
	Nodes []*configtypes.ParsedNode
	// IgnoredSections — секции целого конфига, которые импорт не читает
	// (route/dns/inbounds/experimental). Показываются в превью.
	IgnoredSections []string
	// DisabledNodes — отметки о выключенных нодах с обновлёнными временами
	// (SPEC 094 D4). Отметка ноды, встреченной в этом прогоне, продлевается;
	// вызывающий решает, сохранять ли карту и запускать ли GC — просроченные
	// отметки удаляются только после успешного СЕТЕВОГО обновления.
	DisabledNodes map[string]int64
}

// LoadNodesFromSource loads and processes nodes from a configtypes.ProxySource
// Handles subscriptions, legacy direct links, and connections
// Returns list of parsed nodes with processed tags.
//
// Тонкая обёртка над LoadNodesFromSourceEx для вызывающих, которым нужны
// только узлы.
func LoadNodesFromSource(
	proxySource configtypes.ProxySource,
	tagCounts map[string]int,
	progressCallback func(float64, string),
	subscriptionIndex, totalSubscriptions int,
) ([]*configtypes.ParsedNode, error) {
	res, err := LoadNodesFromSourceEx(proxySource, tagCounts, progressCallback, subscriptionIndex, totalSubscriptions)
	if res == nil {
		return nil, err
	}
	return res.Nodes, err
}

// LoadNodesFromSourceEx — как LoadNodesFromSource, но возвращает и группы,
// импортированные из sing-box конфига (SPEC 094 A5).
func LoadNodesFromSourceEx(
	proxySource configtypes.ProxySource,
	tagCounts map[string]int,
	progressCallback func(float64, string),
	subscriptionIndex, totalSubscriptions int,
) (*SourceLoadResult, error) {
	startTime := time.Now()
	debuglog.DebugLog("LoadNodesFromSource: START source %d/%d at %s",
		subscriptionIndex+1, totalSubscriptions, startTime.Format("15:04:05.000"))

	nodes := make([]*configtypes.ParsedNode, 0)
	nodesFromThisSource := 0
	skippedDueToLimit := 0

	// SPEC 094 A4: секции импортированного конфига, которые парсер не читает.
	// Группы отдельным списком НЕ идут: они рядовые узлы и лежат в nodes.
	var ignoredSections []string

	// Process subscription from Source field
	if proxySource.Source != "" {
		// Check if source is a direct link (legacy format)
		if IsSubscriptionURL(proxySource.Source) {
			// This is a subscription - download and parse
			if progressCallback != nil {
				progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
					fmt.Sprintf("Downloading subscription %d/%d: %s", subscriptionIndex+1, totalSubscriptions, proxySource.Source))
			}

			fetchStartTime := time.Now()
			var content []byte
			var err error

			// SPEC 052: если caller установил cache-hook (Rebuild без сети
			// или Update после refresh), берём оттуда — экономит fetch.
			if LookupCachedBody != nil {
				if cached, ok := LookupCachedBody(proxySource.Source); ok && len(cached) > 0 {
					content = cached
					debuglog.DebugLog("LoadNodesFromSource: Using cached body for subscription %d/%d (%d bytes)",
						subscriptionIndex+1, totalSubscriptions, len(content))
				}
			}
			if content == nil {
				debuglog.DebugLog("LoadNodesFromSource: Fetching subscription %d/%d: %s",
					subscriptionIndex+1, totalSubscriptions, proxySource.Source)
				content, err = FetchSubscription(proxySource.Source)
			}
			fetchDuration := time.Since(fetchStartTime)
			if err != nil {
				debuglog.DebugLog("LoadNodesFromSource: Failed to fetch subscription %d/%d (took %v): %v",
					subscriptionIndex+1, totalSubscriptions, fetchDuration, err)
				debuglog.ErrorLog("Parser: Failed to fetch subscription from %s: %v", proxySource.Source, err)
			} else if len(content) > 0 {
				debuglog.DebugLog("LoadNodesFromSource: Fetched subscription %d/%d: %d bytes in %v",
					subscriptionIndex+1, totalSubscriptions, len(content), fetchDuration)

				if progressCallback != nil {
					progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions)+10.0/float64(totalSubscriptions),
						fmt.Sprintf("Parsing subscription %d/%d: %s", subscriptionIndex+1, totalSubscriptions, proxySource.Source))
				}

				// Parse subscription content: JSON array of Xray configs, or line-by-line URIs
				parseStartTime := time.Now()
				contentStr := string(content)
				contentStr = strings.ReplaceAll(contentStr, "\r\n", "\n")
				contentStr = strings.ReplaceAll(contentStr, "\r", "\n")
				contentStr = strings.TrimSpace(contentStr)

				bodyKind := ClassifySubscriptionBody(contentStr)

				if bodyKind.IsSingbox() {
					// SPEC 094 фаза A: подписка отдала sing-box JSON —
					// одиночный outbound, массив outbound'ов, целый конфиг
					// или массив конфигов. До SPEC 094 такое тело не давало
					// ни одной ноды.
					importRes, err := ParseSingboxBody(contentStr, bodyKind, proxySource.Skip)
					if err != nil {
						debuglog.WarnLog("Parser: sing-box JSON subscription %s: %v", proxySource.Source, err)
					} else {
						debuglog.DebugLog("LoadNodesFromSource: sing-box JSON subscription %d/%d (%s): %d node(s)",
							subscriptionIndex+1, totalSubscriptions, bodyKind, len(importRes.Nodes))
						// SPEC 094 D3: дедуп ДО простановки тегов — иначе
						// MakeTagUnique успеет присвоить дублю тег "…-2",
						// и в конфиг уедет лишний узел с чужим именем.
						importRes.Nodes = dedupNodesByIdentity(importRes.Nodes)

						nodeNum := 0
						accepted := make([]*configtypes.ParsedNode, 0, len(importRes.Nodes))
						for _, node := range importRes.Nodes {
							if nodesFromThisSource >= configtypes.MaxNodesPerSubscription {
								skippedDueToLimit++
								if skippedDueToLimit == 1 {
									debuglog.DebugLog("LoadNodesFromSource: Reached limit of %d nodes for subscription %d/%d",
										configtypes.MaxNodesPerSubscription, subscriptionIndex+1, totalSubscriptions)
								}
								continue
							}
							nodeNum++
							applyTagsToSingboxNode(node, proxySource, nodeNum, tagCounts)
							accepted = append(accepted, node)
							nodesFromThisSource++
						}
						// Узлы-группы ссылаются на теги соседей, а те получили
						// префикс/маску/уникализацию — состав переписывается на
						// итоговые теги. Группа, потерявшая всех членов (лимит,
						// skip-фильтр), отбрасывается: пустой urltest роняет ядро.
						accepted = rebindImportedGroupNodes(accepted)
						nodes = append(nodes, accepted...)
						ignoredSections = importRes.IgnoredSections
					}
					debuglog.DebugLog("LoadNodesFromSource: Parsed subscription %d/%d: %d nodes in %v (%s)",
						subscriptionIndex+1, totalSubscriptions, nodesFromThisSource, time.Since(parseStartTime), bodyKind)
				} else if bodyKind == BodyKindXrayArray {
					arrayNodes, err := ParseNodesFromXrayJSONArray(contentStr, proxySource.Skip)
					if err != nil {
						debuglog.WarnLog("Parser: Xray JSON array subscription %s: %v", proxySource.Source, err)
					} else {
						debuglog.DebugLog("LoadNodesFromSource: Xray JSON array subscription %d/%d: %d node(s)",
							subscriptionIndex+1, totalSubscriptions, len(arrayNodes))

						// SPEC 094 D3: дедуп ВНУТРИ элемента, а не по всей
						// подписке. Провайдеры намеренно переиспользуют один
						// сервер в разных элементах: тот же адрес приезжает и
						// как «🇩🇪 Германия», и внутри пула «Авто | Лучший
						// сервер». Для пользователя это разные записи с разным
						// смыслом, и схлопывание стирало половину стран из
						// списка. Дедуп внутри элемента выполняет сам
						// ParseNodesFromXrayJSONArray.
						nodeNum := 0
						acceptedXray := make([]*configtypes.ParsedNode, 0, len(arrayNodes))
						for _, node := range arrayNodes {
							if nodesFromThisSource >= configtypes.MaxNodesPerSubscription {
								skippedDueToLimit++
								if skippedDueToLimit == 1 {
									debuglog.DebugLog("LoadNodesFromSource: Reached limit of %d nodes for subscription %d/%d",
										configtypes.MaxNodesPerSubscription, subscriptionIndex+1, totalSubscriptions)
								}
								continue
							}
							nodeNum++
							applyTagsToXrayNode(node, proxySource, nodeNum, tagCounts)
							acceptedXray = append(acceptedXray, node)
							nodesFromThisSource++
						}
						// Узлы-группы (§322) ссылаются на теги соседей, а те
						// получили префикс/маску/уникализацию — состав
						// переписывается на итоговые теги. Без этого группа с
						// tag_prefix указывала в пустоту: `sing-box check`
						// такое пропускает, но в рантайме группа мертва.
						acceptedXray = rebindImportedGroupNodes(acceptedXray)
						nodes = append(nodes, acceptedXray...)
					}
					debuglog.DebugLog("LoadNodesFromSource: Parsed subscription %d/%d: %d nodes in %v (Xray JSON array)",
						subscriptionIndex+1, totalSubscriptions, nodesFromThisSource, time.Since(parseStartTime))
				} else {
					subscriptionLines := strings.Split(contentStr, "\n")
					debuglog.DebugLog("LoadNodesFromSource: Parsing subscription %d/%d: %d lines",
						subscriptionIndex+1, totalSubscriptions, len(subscriptionLines))

					// SPEC 094 D3: дедуп внутри источника. Проверка идёт до
					// простановки тегов — MakeTagUnique иначе успел бы выдать
					// дублю тег "…-2", и в конфиг уехал бы лишний узел.
					// Инкрементально, а не по собранному списку: подписка на
					// 3000 нод не должна дважды лежать в памяти целиком.
					seenIdentities := make(map[string]string)

					lineCount := 0
					for _, subLine := range subscriptionLines {
						subLine = NormalizeSubscriptionTextLine(subLine)
						if subLine == "" {
							continue
						}
						lineCount++

						if nodesFromThisSource >= configtypes.MaxNodesPerSubscription {
							skippedDueToLimit++
							if skippedDueToLimit == 1 {
								debuglog.DebugLog("LoadNodesFromSource: Reached limit of %d nodes for subscription %d/%d",
									configtypes.MaxNodesPerSubscription, subscriptionIndex+1, totalSubscriptions)
							}
							continue
						}

						nodeStartTime := time.Now()
						node, err := ParseNode(subLine, proxySource.Skip)
						if err != nil {
							debuglog.DebugLog("LoadNodesFromSource: Failed to parse node %d from subscription %d/%d (took %v): %v",
								lineCount, subscriptionIndex+1, totalSubscriptions, time.Since(nodeStartTime), err)
							debuglog.WarnLog("Parser: Failed to parse node from subscription %s: %v", proxySource.Source, err)
							continue
						}

						if node != nil {
							if NodeIdentityHashFunc != nil {
								if hash := NodeIdentityHashFunc(node); hash != "" {
									if firstTag, dup := seenIdentities[hash]; dup {
										debuglog.DebugLog("Parser: dedup: %q duplicates %q (same identity) — dropped", node.Tag, firstTag)
										continue
									}
									seenIdentities[hash] = node.Tag
								}
							}

							// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
							node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodesFromThisSource+1)
							node.Tag = textnorm.NormalizeProxyDisplay(node.Tag)
							node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
							nodes = append(nodes, node)
							nodesFromThisSource++
							if nodesFromThisSource%50 == 0 {
								debuglog.DebugLog("LoadNodesFromSource: Parsed %d nodes from subscription %d/%d (elapsed: %v)",
									nodesFromThisSource, subscriptionIndex+1, totalSubscriptions, time.Since(parseStartTime))
							}
						}
					}
					debuglog.DebugLog("LoadNodesFromSource: Parsed subscription %d/%d: %d nodes in %v (processed %d lines)",
						subscriptionIndex+1, totalSubscriptions, nodesFromThisSource, time.Since(parseStartTime), lineCount)
				}
			}
		} else if IsDirectLink(proxySource.Source) {
			// Legacy format: direct link in Source
			debuglog.DebugLog("LoadNodesFromSource: Processing direct link in Source field for %d/%d",
				subscriptionIndex+1, totalSubscriptions)
			if progressCallback != nil {
				progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
					fmt.Sprintf("Parsing direct link %d/%d", subscriptionIndex+1, totalSubscriptions))
			}

			if nodesFromThisSource < configtypes.MaxNodesPerSubscription {
				parseStartTime := time.Now()
				src := NormalizeSubscriptionTextLine(proxySource.Source)
				node, err := ParseNode(src, proxySource.Skip)
				if err != nil {
					debuglog.DebugLog("LoadNodesFromSource: Failed to parse direct link (took %v): %v",
						time.Since(parseStartTime), err)
					debuglog.WarnLog("Parser: Failed to parse direct link: %v", err)
				} else if node != nil {
					// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
					node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodesFromThisSource+1)
					node.Tag = textnorm.NormalizeProxyDisplay(node.Tag)
					node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
					nodes = append(nodes, node)
					nodesFromThisSource++
					debuglog.DebugLog("LoadNodesFromSource: Parsed direct link in %v", time.Since(parseStartTime))
				}
			} else {
				skippedDueToLimit++
			}
		}
	}

	// Process direct links from Connections field
	connectionsStartTime := time.Now()
	debuglog.DebugLog("LoadNodesFromSource: Processing %d direct connections for source %d/%d",
		len(proxySource.Connections), subscriptionIndex+1, totalSubscriptions)
	for connIndex, connection := range proxySource.Connections {
		connection = NormalizeSubscriptionTextLine(connection)
		if connection == "" {
			continue
		}

		if !IsDirectLink(connection) {
			debuglog.DebugLog("LoadNodesFromSource: Invalid direct link format in connections %d/%d: %s",
				connIndex+1, len(proxySource.Connections), connection)
			debuglog.WarnLog("Parser: Invalid direct link format in connections: %s", connection)
			continue
		}

		if progressCallback != nil {
			progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
				fmt.Sprintf("Parsing direct link %d/%d (connection %d)", subscriptionIndex+1, totalSubscriptions, connIndex+1))
		}

		if nodesFromThisSource >= configtypes.MaxNodesPerSubscription {
			skippedDueToLimit++
			continue
		}

		parseStartTime := time.Now()
		node, err := ParseNode(connection, proxySource.Skip)
		if err != nil {
			debuglog.DebugLog("LoadNodesFromSource: Failed to parse connection %d/%d (took %v): %v",
				connIndex+1, len(proxySource.Connections), time.Since(parseStartTime), err)
			debuglog.WarnLog("Parser: Failed to parse direct link from connections: %v", err)
			continue
		}

		if node != nil {
			// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
			node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodesFromThisSource+1)
			node.Tag = textnorm.NormalizeProxyDisplay(node.Tag)
			node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
			nodes = append(nodes, node)
			nodesFromThisSource++
		}
	}
	if len(proxySource.Connections) > 0 {
		debuglog.DebugLog("LoadNodesFromSource: Processed %d connections in %v",
			len(proxySource.Connections), time.Since(connectionsStartTime))
	}

	if skippedDueToLimit > 0 {
		debuglog.DebugLog("LoadNodesFromSource: Source %d/%d exceeded limit, skipped %d nodes",
			subscriptionIndex+1, totalSubscriptions, skippedDueToLimit)
		debuglog.WarnLog("Parser: Source exceeded limit of %d nodes. Skipped %d additional nodes.",
			configtypes.MaxNodesPerSubscription, skippedDueToLimit)
	}

	// SPEC 077: apply the source-level detour to every node it produced, so the
	// generator emits "detour":"<tag>" on each. Skipped for WireGuard (endpoint,
	// not a dial-based outbound) and for nodes that already carry an Xray Jump
	// (the subscription declared its own chain — that wins). The tag is validated
	// (dangling/cycle/self) later in the generator, where the full tag set is
	// known; here we only stamp it.
	applySourceDetour(nodes, proxySource.DetourTag)

	// SPEC 094 D4: узлы, выключенные пользователем, не попадают в конфиг.
	// Отметки живут по хешу идентичности, поэтому переживают переименование
	// ноды провайдером и смену её позиции в подписке.
	nodes, refreshedDisabled := filterDisabledNodes(nodes, proxySource.DisabledNodes, time.Now())

	totalDuration := time.Since(startTime)
	debuglog.DebugLog("LoadNodesFromSource: END source %d/%d (total duration: %v, nodes: %d)",
		subscriptionIndex+1, totalSubscriptions, totalDuration, len(nodes))
	return &SourceLoadResult{
		Nodes:           nodes,
		IgnoredSections: ignoredSections,
		DisabledNodes:   refreshedDisabled,
	}, nil
}

// applyTagsToSingboxNode применяет к импортированному узлу те же правила тегов,
// что и к узлам из URI-подписки: префикс/постфикс/маска, нормализация,
// уникализация. Цепочечные хопы получают производные теги, чтобы не
// столкнуться с тегами других источников.
func applyTagsToSingboxNode(
	node *configtypes.ParsedNode,
	proxySource configtypes.ProxySource,
	nodeNum int,
	tagCounts map[string]int,
) {
	if node == nil {
		return
	}
	// Исходный тег нужен группам, чтобы переписать состав на итоговые теги.
	// Comment для этого использовать нельзя — его читают skip-фильтры.
	node.SourceTag = node.Tag

	node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodeNum)
	node.Tag = textnorm.NormalizeProxyDisplay(node.Tag)
	node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
	if node.Outbound != nil {
		node.Outbound["tag"] = node.Tag
	}

	for hopIdx, hop := range node.Chain {
		if hop == nil {
			continue
		}
		hop.Tag = MakeTagUnique(fmt.Sprintf("%s_hop%d", node.Tag, hopIdx+1), tagCounts, "Parser")
		if hop.Outbound != nil {
			hop.Outbound["tag"] = hop.Tag
		}
	}
	// Каждый хоп дозванивается через следующий; последний идёт напрямую.
	for hopIdx, hop := range node.Chain {
		if hop == nil || hop.Outbound == nil {
			continue
		}
		if hopIdx+1 < len(node.Chain) && node.Chain[hopIdx+1] != nil {
			hop.Outbound["detour"] = node.Chain[hopIdx+1].Tag
		} else {
			delete(hop.Outbound, "detour")
		}
	}
	if len(node.Chain) > 0 && node.Chain[0] != nil && node.Outbound != nil {
		node.Outbound["detour"] = node.Chain[0].Tag
	}
	node.SyncJumpFromChain()
}

// rebindImportedGroupNodes переписывает состав узлов-групп с исходных тегов
// конфига на итоговые теги узлов (после префикса/маски/уникализации).
//
// Соответствие берётся из node.SourceTag, куда applyTagsToSingboxNode кладёт
// исходный тег. Член, для которого узла не нашлось, выбрасывается; группа,
// оставшаяся без членов, удаляется целиком — пустой urltest роняет старт ядра.
//
// Узел, не прошедший лимит MaxNodesPerSubscription или отсечённый skip-фильтром,
// в списке отсутствует и потому из состава выпадает: ссылка на неэмитированный
// тег отвергается ядром.
//
// Возвращает отфильтрованный список узлов.
func rebindImportedGroupNodes(nodes []*configtypes.ParsedNode) []*configtypes.ParsedNode {
	finalByOriginal := make(map[string]string, len(nodes))
	for _, node := range nodes {
		if node == nil || node.SourceTag == "" {
			continue
		}
		finalByOriginal[node.SourceTag] = node.Tag
	}

	kept := make([]*configtypes.ParsedNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Scheme != configtypes.SchemeGroup || node.Outbound == nil {
			kept = append(kept, node)
			continue
		}

		members := make([]interface{}, 0)
		switch raw := node.Outbound[configtypes.GroupMembersKey].(type) {
		case []interface{}:
			for _, item := range raw {
				if memberTag, ok := item.(string); ok {
					if finalTag, ok := finalByOriginal[memberTag]; ok {
						members = append(members, finalTag)
					}
				}
			}
		}

		if len(members) == 0 {
			debuglog.WarnLog("Parser: singbox import: group %q lost all members after tag rewrite — skipped", node.Tag)
			continue
		}
		node.Outbound[configtypes.GroupMembersKey] = members

		if def, ok := node.Outbound["default"].(string); ok {
			if finalTag, ok := finalByOriginal[def]; ok {
				node.Outbound["default"] = finalTag
			} else {
				delete(node.Outbound, "default")
			}
		}
		kept = append(kept, node)
	}
	return kept
}

// applySourceDetour stamps node.Outbound["detour"] = detourTag on every eligible
// node (SPEC 077). No-op when detourTag is empty. WireGuard nodes and nodes with
// an Xray Jump are left untouched (see LoadNodesFromSource for the rationale).
func applySourceDetour(nodes []*configtypes.ParsedNode, detourTag string) {
	detourTag = strings.TrimSpace(detourTag)
	if detourTag == "" {
		return
	}
	for _, node := range nodes {
		if node == nil || node.Scheme == "wireguard" {
			continue
		}
		if node.Jump != nil {
			debuglog.DebugLog("applySourceDetour: node %q has an Xray Jump — source detour %q not applied", node.Tag, detourTag)
			continue
		}
		if node.Outbound == nil {
			node.Outbound = map[string]interface{}{}
		}
		node.Outbound["detour"] = detourTag
	}
}

// applyTagsToXrayNode applies tag_prefix/tag_postfix/tag_mask and MakeTagUnique to main and jump tags.
func applyTagsToXrayNode(node *configtypes.ParsedNode, proxySource configtypes.ProxySource, nodeNum int, tagCounts map[string]int) {
	// SPEC 094: исходный тег нужен узлам-группам, чтобы после префикса/маски
	// переписать состав на итоговые теги. Без этого группа из Xray-подписки
	// с tag_prefix ссылалась на несуществующие теги: `sing-box check` такое
	// пропускает (существование членов он не проверяет), а в рантайме группа
	// мертва.
	node.SourceTag = node.Tag

	if node.Jump != nil {
		saved := node.Tag
		node.Tag = node.Jump.Tag
		node.Jump.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodeNum)
		node.Tag = saved
	}
	node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodeNum)
	node.Tag = textnorm.NormalizeProxyDisplay(node.Tag)
	node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
	if node.Jump != nil {
		node.Jump.Tag = textnorm.NormalizeProxyDisplay(node.Jump.Tag)
		node.Jump.Tag = MakeTagUnique(node.Jump.Tag, tagCounts, "Parser")
		if node.Jump.Outbound != nil {
			node.Jump.Outbound["tag"] = node.Jump.Tag
		}
	}
	if node.Outbound != nil {
		node.Outbound["tag"] = node.Tag
	}
}

// applyTagPrefixPostfix applies prefix and postfix to a node tag if specified in ProxySource.
// If tagMask is set, it replaces the entire tag and ignores prefix/postfix.
// Supports variable substitution in prefix, postfix, and mask.
// Returns the modified tag.
func applyTagPrefixPostfix(node *configtypes.ParsedNode, tagPrefix, tagPostfix, tagMask string, nodeNum int) string {
	// If tag_mask is set, use it to replace the entire tag (ignores prefix/postfix)
	if tagMask != "" {
		return replaceTagVariables(tagMask, node, nodeNum)
	}

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
