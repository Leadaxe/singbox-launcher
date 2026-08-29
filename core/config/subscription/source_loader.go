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

// nodeIdentity — идентичность узла через хук, с встроенным запасным правилом.
func nodeIdentity(node *configtypes.ParsedNode) string {
	if node == nil {
		return ""
	}
	if NodeIdentityFunc != nil {
		return NodeIdentityFunc(node)
	}
	if node.Scheme == configtypes.SchemeGroup {
		return ""
	}
	if id := strings.TrimSpace(node.IdentityTag); id != "" {
		return id
	}
	return strings.TrimSpace(node.Tag)
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

	// ParseFailures — КОМПАКТНЫЙ список причин, по которым разбор отбраковал
	// содержимое источника: сетевая ошибка фетча, битые элементы Xray-массива,
	// нечитаемое тело.
	//
	// Заполняется всегда, когда причина была, — даже если узлы всё-таки есть:
	// «половина подписки протухла» тоже стоит показать. Решение «показывать
	// или нет» принимает адресат (отчёт сборки показывает только у источника,
	// не давшего ни одного узла), а не разбор: разбор не знает, чем кончится
	// сборка.
	//
	// SPEC 113-A не затрагивается: это только видимость. Достоверность разбора
	// считается прежним способом, и наличие причин на неё не влияет.
	ParseFailures []string
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

	// SPEC 112: счётчик уникализации ИДЕНТИЧНОСТЕЙ — свой на источник, в
	// отличие от tagCounts (тот общий на весь конфиг: конфиговые теги обязаны
	// быть уникальны глобально, идентичность — только внутри источника).
	idCounts := make(map[string]int)

	// SPEC 112-B: дедуп записей ПО ПОДПИСИ СОДЕРЖИМОГО — свой на источник, как
	// и idCounts. Опрашивается ДО простановки тегов (SPEC 094 D3): пропусти
	// проверку, и дубль сперва получил бы уникализованный тег «X-2», а с ним
	// и собственную идентичность — снять его отметку стало бы нечем.
	dedup := newSourceDedup()

	// SPEC 094 A4: секции импортированного конфига, которые парсер не читает.
	// Группы отдельным списком НЕ идут: они рядовые узлы и лежат в nodes.
	var ignoredSections []string

	// SPEC 115: причины отбраковки — компактно, для отчёта сборки. Копятся
	// здесь, а не в лог, потому что адресат у них пользователь: источник,
	// разобравшийся в ноль узлов, до этого не сообщал о себе ничего.
	rejected := &ParseFailureReasons{}

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

			{
				debuglog.DebugLog("LoadNodesFromSource: Fetching subscription %d/%d: %s",
					subscriptionIndex+1, totalSubscriptions, proxySource.Source)
				content, err = FetchSubscription(proxySource.Source)
			}
			fetchDuration := time.Since(fetchStartTime)
			if err != nil {
				debuglog.DebugLog("LoadNodesFromSource: Failed to fetch subscription %d/%d (took %v): %v",
					subscriptionIndex+1, totalSubscriptions, fetchDuration, err)
				debuglog.ErrorLog("Parser: Failed to fetch subscription from %s: %v", proxySource.Source, err)
				// Сетевая ошибка идёт наверх КАК ЕСТЬ: сокращать «connection
				// refused» до «источник не загрузился» значило бы отнять у
				// пользователя ровно то, по чему он отличает сбой провайдера
				// от собственного файрвола.
				rejected.Add(fmt.Sprintf("fetch failed: %v", err))
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

				// SPEC 103 B11: подписка отдала wg-quick .conf. Тело сводится к
				// каноническим wireguard://-URI ДО ветвления, и дальше идёт тем
				// же путём, что URI-список: разбор, AWG-поля и кламп MTU уже
				// реализованы в parseWireGuardURI, а второй разбор INI
				// разъехался бы с первым при первой же правке.
				if bodyKind == BodyKindVPNLink {
					// SPEC 103 §9.B12: тело — Amnezia-профиль. Импортируются
					// ВСЕ WG/AWG-контейнеры: профиль с несколькими локациями —
					// штатный случай, и терять их незачем (одиночный ParseNode
					// вынужденно берёт один, потому что отдаёт одну ноду).
					vpnNodes, skippedContainers, vpnErr := ParseAmneziaVPNLinkAll(contentStr, proxySource.Skip)
					if vpnErr != nil {
						debuglog.WarnLog("Parser: vpn:// subscription %s: %v", proxySource.Source, vpnErr)
					} else {
						if skippedContainers > 0 {
							debuglog.WarnLog("Parser: vpn:// subscription %s: %d container(s) skipped",
								proxySource.Source, skippedContainers)
						}
						nodeNum := 0
						for _, node := range vpnNodes {
							if nodesFromThisSource >= configtypes.MaxNodesPerSubscription {
								skippedDueToLimit++
								continue
							}
							if !dedup.accept(node) {
								continue
							}
							nodeNum++
							applyTagsToSingboxNode(node, proxySource, nodeNum, tagCounts, idCounts)
							nodes = append(nodes, node)
							nodesFromThisSource++
						}
						debuglog.DebugLog("LoadNodesFromSource: vpn:// subscription %d/%d: %d node(s)",
							subscriptionIndex+1, totalSubscriptions, len(vpnNodes))
					}
					// Тело обработано целиком — построчный разбор не нужен.
					contentStr = ""
					bodyKind = BodyKindURIList
				}

				if bodyKind == BodyKindWGConf {
					wgURIs, skippedBlocks := WGConfBodyToURIs(contentStr)
					if skippedBlocks > 0 {
						debuglog.WarnLog("Parser: wg-quick subscription %s: %d block(s) skipped (no [Peer] endpoint)",
							proxySource.Source, skippedBlocks)
					}
					if len(wgURIs) == 0 {
						debuglog.WarnLog("Parser: wg-quick subscription %s: no usable [Interface] block",
							proxySource.Source)
					}
					debuglog.DebugLog("LoadNodesFromSource: wg-quick subscription %d/%d: %d URI(s)",
						subscriptionIndex+1, totalSubscriptions, len(wgURIs))
					contentStr = strings.Join(wgURIs, "\n")
					bodyKind = BodyKindURIList
				}

				if bodyKind.IsSingbox() {
					// SPEC 094 фаза A: подписка отдала sing-box JSON —
					// одиночный outbound, массив outbound'ов, целый конфиг
					// или массив конфигов. До SPEC 094 такое тело не давало
					// ни одной ноды.
					importRes, err := ParseSingboxBody(contentStr, bodyKind, proxySource.Skip)
					if err != nil {
						debuglog.WarnLog("Parser: sing-box JSON subscription %s: %v", proxySource.Source, err)
						rejected.Add(fmt.Sprintf("sing-box JSON body rejected: %v", err))
					} else {
						debuglog.DebugLog("LoadNodesFromSource: sing-box JSON subscription %d/%d (%s): %d node(s)",
							subscriptionIndex+1, totalSubscriptions, bodyKind, len(importRes.Nodes))
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
							// Дубль отбрасывается ДО простановки тегов; группа
							// узлов ключа не имеет и проходит всегда — её состав
							// переписывается ниже на выживших.
							if !dedup.accept(node) {
								continue
							}
							nodeNum++
							applyTagsToSingboxNode(node, proxySource, nodeNum, tagCounts, idCounts)
							accepted = append(accepted, node)
							nodesFromThisSource++
						}
						// Узлы-группы ссылаются на теги соседей, а те получили
						// префикс/маску/уникализацию — состав переписывается на
						// итоговые теги. Группа, потерявшая всех членов (лимит,
						// skip-фильтр), отбрасывается: пустой urltest роняет ядро.
						accepted = rebindImportedGroupNodes(accepted, dedup.collapsedTags())
						nodes = append(nodes, accepted...)
						ignoredSections = importRes.IgnoredSections
					}
					debuglog.DebugLog("LoadNodesFromSource: Parsed subscription %d/%d: %d nodes in %v (%s)",
						subscriptionIndex+1, totalSubscriptions, nodesFromThisSource, time.Since(parseStartTime), bodyKind)
				} else if bodyKind == BodyKindXrayArray {
					arrayNodes, xrayReasons, err := ParseNodesFromXrayJSONArrayEx(contentStr, proxySource.Skip)
					rejected.AddAll(xrayReasons)
					if err != nil {
						debuglog.WarnLog("Parser: Xray JSON array subscription %s: %v", proxySource.Source, err)
						rejected.Add(fmt.Sprintf("Xray JSON array body rejected: %v", err))
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
							applyTagsToXrayNode(node, proxySource, nodeNum, tagCounts, idCounts)
							acceptedXray = append(acceptedXray, node)
							nodesFromThisSource++
						}
						// Узлы-группы (§322) ссылаются на теги соседей, а те
						// получили префикс/маску/уникализацию — состав
						// переписывается на итоговые теги. Без этого группа с
						// tag_prefix указывала в пустоту: `sing-box check`
						// такое пропускает, но в рантайме группа мертва.
						// Xray-массив дедупится внутри ParseNodesFromXrayJSONArray
						// (ownership по подписи содержимого), состав групп там же
						// и резолвится — карта дедупа источника здесь пуста.
						acceptedXray = rebindImportedGroupNodes(acceptedXray, dedup.collapsedTags())
						nodes = append(nodes, acceptedXray...)
					}
					debuglog.DebugLog("LoadNodesFromSource: Parsed subscription %d/%d: %d nodes in %v (Xray JSON array)",
						subscriptionIndex+1, totalSubscriptions, nodesFromThisSource, time.Since(parseStartTime))
				} else {
					subscriptionLines := strings.Split(contentStr, "\n")
					debuglog.DebugLog("LoadNodesFromSource: Parsing subscription %d/%d: %d lines",
						subscriptionIndex+1, totalSubscriptions, len(subscriptionLines))

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
							// Дубль отбрасывается ДО applyURINodeTags — иначе он
							// получил бы тег «X-2» и уехал под чужим именем.
							if !dedup.accept(node) {
								continue
							}
							// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
							applyURINodeTags(node, proxySource, nodesFromThisSource+1, tagCounts, idCounts)
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
				} else if node != nil && dedup.accept(node) {
					// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
					applyURINodeTags(node, proxySource, nodesFromThisSource+1, tagCounts, idCounts)
					nodes = append(nodes, node)
					nodesFromThisSource++
					debuglog.DebugLog("LoadNodesFromSource: Parsed direct link in %v", time.Since(parseStartTime))
				}
			} else {
				skippedDueToLimit++
			}
		}
	}

	// Manual config_json (server-source): готовый sing-box объект вместо URI.
	// Приоритетнее Connections: раз пользователь сохранил ручной JSON, URI
	// намеренно игнорируется — он мог вообще не парситься (протокол без
	// схемы/парсера), в этом весь смысл поля. Tag перештамповывается тем же
	// путём, что у остальных нод (mask=Label для server-source).
	connections := proxySource.Connections
	if len(proxySource.ConfigJSON) > 0 {
		node, err := NodeFromManualConfigJSON(proxySource.ConfigJSON)
		if err != nil {
			debuglog.WarnLog("Parser: manual config_json for source %d/%d: %v",
				subscriptionIndex+1, totalSubscriptions, err)
		} else if nodesFromThisSource < configtypes.MaxNodesPerSubscription && dedup.accept(node) {
			applyURINodeTags(node, proxySource, nodesFromThisSource+1, tagCounts, idCounts)
			nodes = append(nodes, node)
			nodesFromThisSource++
			debuglog.DebugLog("LoadNodesFromSource: manual config_json node %q (type %s) for source %d/%d",
				node.Tag, node.Scheme, subscriptionIndex+1, totalSubscriptions)
		}
		connections = nil
	}

	// Process direct links from Connections field
	connectionsStartTime := time.Now()
	debuglog.DebugLog("LoadNodesFromSource: Processing %d direct connections for source %d/%d",
		len(connections), subscriptionIndex+1, totalSubscriptions)
	for connIndex, connection := range connections {
		connection = NormalizeSubscriptionTextLine(connection)
		if connection == "" {
			continue
		}

		if !IsDirectLink(connection) {
			debuglog.DebugLog("LoadNodesFromSource: Invalid direct link format in connections %d/%d: %s",
				connIndex+1, len(connections), connection)
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
				connIndex+1, len(connections), time.Since(parseStartTime), err)
			debuglog.WarnLog("Parser: Failed to parse direct link from connections: %v", err)
			continue
		}

		if node != nil && dedup.accept(node) {
			// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
			applyURINodeTags(node, proxySource, nodesFromThisSource+1, tagCounts, idCounts)
			nodes = append(nodes, node)
			nodesFromThisSource++
		}
	}
	if len(connections) > 0 {
		debuglog.DebugLog("LoadNodesFromSource: Processed %d connections in %v",
			len(connections), time.Since(connectionsStartTime))
	}

	if skippedDueToLimit > 0 {
		debuglog.DebugLog("LoadNodesFromSource: Source %d/%d exceeded limit, skipped %d nodes",
			subscriptionIndex+1, totalSubscriptions, skippedDueToLimit)
		debuglog.WarnLog("Parser: Source exceeded limit of %d nodes. Skipped %d additional nodes.",
			configtypes.MaxNodesPerSubscription, skippedDueToLimit)
	}

	// Итог дедупа — до строки END, чтобы в логе он читался как часть разбора,
	// а не как что-то, случившееся после него.
	dedup.logSummary(proxySource.Source)

	totalDuration := time.Since(startTime)
	debuglog.DebugLog("LoadNodesFromSource: END source %d/%d (total duration: %v, nodes: %d)",
		subscriptionIndex+1, totalSubscriptions, totalDuration, len(nodes))
	// Хук зовётся ДО возврата и одинаково для обеих обёрток: тонкая
	// LoadNodesFromSource отдаёт наверх только узлы, и без него причины
	// доезжали бы лишь до тех вызывающих, кто зовёт Ex-форму.
	reportParseFailures(proxySource, rejected.List())
	return &SourceLoadResult{
		Nodes:           nodes,
		IgnoredSections: ignoredSections,
		ParseFailures:   rejected.List(),
	}, nil
}

// applyURINodeTags штампует узлу из URI идентичность и итоговый тег.
//
// Порядок обязателен (SPEC 112, ловушка «порядок стемпинга тегов»):
// идентичность снимается с СЫРОГО тега — до префикса/маски/глобальной
// уникализации, — иначе правка tag_prefix источника уводила бы отметки
// выключения из-под пользователя.
func applyURINodeTags(
	node *configtypes.ParsedNode,
	proxySource configtypes.ProxySource,
	nodeNum int,
	tagCounts map[string]int,
	idCounts map[string]int,
) {
	if node == nil {
		return
	}
	StampNodeIdentity(node, idCounts)
	node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, nodeNum)
	node.Tag = textnorm.NormalizeProxyDisplay(node.Tag)
	node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
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
	idCounts map[string]int,
) {
	if node == nil {
		return
	}
	// Исходный тег нужен группам, чтобы переписать состав на итоговые теги.
	// Comment для этого использовать нельзя — его читают skip-фильтры.
	node.SourceTag = node.Tag

	// SPEC 112: идентичность — сырой тег, уникализированный в пределах
	// источника. Снимается ДО префикса/маски и ДО глобальной уникализации.
	// SourceTag для этого не годится: он не уникализирован, и два узла с
	// одинаковым именем в импортированном конфиге получили бы один ключ.
	StampNodeIdentity(node, idCounts)

	node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, nodeNum)
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
// collapsedInto — «исходный тег дубля → исходный тег выжившего» от дедупа
// (SPEC 113-A §4). Член, схлопнутый дедупом, перепривязывается на ВЫЖИВШУЮ
// копию, а не выпадает: группа из одних дублей иначе теряла весь состав и
// удалялась, хотя её узлы живы под другими именами. Повторы после
// перепривязки схлопываются — дубль тега в outbounds ядро отвергает.
//
// Возвращает отфильтрованный список узлов.
func rebindImportedGroupNodes(
	nodes []*configtypes.ParsedNode,
	collapsedInto map[string]string,
) []*configtypes.ParsedNode {
	finalByOriginal := make(map[string]string, len(nodes))
	for _, node := range nodes {
		if node == nil || node.SourceTag == "" {
			continue
		}
		finalByOriginal[node.SourceTag] = node.Tag
	}

	// resolveMember — итоговый тег члена: свой, а если узел схлопнут дедупом —
	// тег выжившей копии.
	resolveMember := func(memberTag string) (string, bool) {
		if finalTag, ok := finalByOriginal[memberTag]; ok {
			return finalTag, true
		}
		if survivor, ok := collapsedInto[memberTag]; ok {
			if finalTag, ok := finalByOriginal[survivor]; ok {
				return finalTag, true
			}
		}
		return "", false
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
			seen := make(map[string]struct{}, len(raw))
			for _, item := range raw {
				if memberTag, ok := item.(string); ok {
					finalTag, ok := resolveMember(memberTag)
					if !ok {
						continue
					}
					if _, dup := seen[finalTag]; dup {
						continue
					}
					seen[finalTag] = struct{}{}
					members = append(members, finalTag)
				}
			}
		}

		if len(members) == 0 {
			debuglog.WarnLog("Parser: singbox import: group %q lost all members after tag rewrite — skipped", node.Tag)
			continue
		}
		node.Outbound[configtypes.GroupMembersKey] = members

		if def, ok := node.Outbound["default"].(string); ok {
			if finalTag, ok := resolveMember(def); ok {
				node.Outbound["default"] = finalTag
			} else {
				delete(node.Outbound, "default")
			}
		}
		kept = append(kept, node)
	}
	return kept
}

// applyTagsToXrayNode applies tag_prefix/tag_postfix/tag_mask and MakeTagUnique to main and jump tags.
func applyTagsToXrayNode(node *configtypes.ParsedNode, proxySource configtypes.ProxySource, nodeNum int, tagCounts map[string]int, idCounts map[string]int) {
	// SPEC 094: исходный тег нужен узлам-группам, чтобы после префикса/маски
	// переписать состав на итоговые теги. Без этого группа из Xray-подписки
	// с tag_prefix ссылалась на несуществующие теги: `sing-box check` такое
	// пропускает (существование членов он не проверяет), а в рантайме группа
	// мертва.
	node.SourceTag = node.Tag

	// SPEC 112: идентичность — сырой тег до префикса/маски, уникализированный
	// в пределах источника.
	StampNodeIdentity(node, idCounts)

	if node.Jump != nil {
		saved := node.Tag
		node.Tag = node.Jump.Tag
		node.Jump.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, nodeNum)
		node.Tag = saved
	}
	node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, nodeNum)
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
