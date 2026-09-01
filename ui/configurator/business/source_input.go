// File source_input.go — ОБЩЕЕ ЯДРО разбора вставленного текста в узлы
// (SPEC 116 этап 3, W6; дыра Д6).
//
// # Зачем вынесено
//
// До W6 разбор текста жил внутри `AppendURLsToSources` и умел ровно одно:
// класть каждый разобранный узел ОТДЕЛЬНЫМ Source в корень `model.Sources`.
// Наполнение папки (цель 2) хочет тот же текст, тот же классификатор и ту же
// материализацию, но с другим адресом назначения — `Source.Nodes` папки.
//
// Второй разбор здесь запрещён прямым правилом кампании («эмиттер и парсер
// ходят парой»): две реализации одного разбора разъезжаются на первой же
// правке — ровно так уже терялись схемы masque/anytls/ssh. Поэтому текст
// превращается в узлы РОВНО ОДИН раз, здесь, а «куда положить» решает
// вызывающий: корень (`AppendURLsToSources`) или папка
// (`AppendNodesToFolder`).
//
// # Что ядро НЕ делает
//
// Оно не трогает модель, не поднимает ревизию и не уникализирует теги: тег
// уникализируется в пределах КОНТЕЙНЕРА, а контейнер ядру неизвестен (у корня
// одно пространство имён, у каждой папки своё). Это делает вызывающий —
// корневая ветка своей дедупой по URI, папка через `uniqueTagIn`.
package business

import (
	"fmt"
	"time"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

// parsedSourceInput — результат разбора одного вставленного текста.
//
// Подписки и узлы разделены намеренно: подписка — это КОНТЕЙНЕР со своим
// URL и расписанием, узлом она не становится ни при каких условиях. Папка
// вложенных контейнеров не держит (features/sources.md: вложенных папок нет),
// поэтому её ветка на непустой `Subscriptions` обязана сказать об этом вслух,
// а не проглотить строку молча.
type parsedSourceInput struct {
	// Subscriptions — строки, распознанные как подписочные URL.
	Subscriptions []string
	// Nodes — материализованные узлы (server): share-URI, wg-INI и sing-box
	// JSON уже разобраны, тело лежит в Body, происхождение — в Origin.
	// Сырые теги НЕ уникализированы (см. шапку файла).
	Nodes []corestate.Node
	// URIOf[i] — исходный share-URI узла Nodes[i], либо "" для JSON-узла.
	// Нужен корневой ветке для дедупа по URI; папке — нет (там дедуп по тегу).
	URIOf []string
	// Unnamed[i] — у узла Nodes[i] не было собственного имени (ни #fragment,
	// ни tag в JSON), и тег ему выдан заглушкой. Такому узлу вызывающий
	// вправе дать имя извне — например, из имени импортируемого файла
	// (features/sources.md §«Наполнение папки» п.1).
	Unnamed []bool
}

// parseSourceInput — текст → узлы. Единственный разбор на все пути Add.
//
// Порядок веток дословно повторяет прежний `AppendURLsToSources`: sing-box
// JSON выкусывается ДО построчного классификатора (документ многострочный, и
// цикл по строкам не нашёл бы в нём ни одной ссылки), и только «не JSON»
// уходит в классификатор строк.
//
// `fallbackIndex` — база нумерации заглушечных тегов (`server-N`): у корня она
// исторически считается от длины `model.Sources`, у папки — от длины её
// `Nodes`. Само правило «безымянному узлу выдаётся server-N» общее, поэтому
// живёт здесь, а не дублируется в двух вызывающих.
func parseSourceInput(input string, fallbackIndex int) (*parsedSourceInput, error) {
	if input == "" {
		return nil, fmt.Errorf("input is empty")
	}

	// isJSON отделяет «не JSON» от «битый JSON»: второе обязано дойти до
	// пользователя ошибкой, а не общим «no valid URLs to add».
	jsonNodes, isJSON, jsonErr := carveSingboxJSON(input)
	if jsonErr != nil {
		return nil, jsonErr
	}

	res := &parsedSourceInput{}
	var conns, rawOf []string
	if !isJSON {
		res.Subscriptions, conns, rawOf = classifyInputLines(input, silentTiming{})
	}
	if len(res.Subscriptions) == 0 && len(conns) == 0 && len(jsonNodes) == 0 {
		return nil, fmt.Errorf("no valid URLs to add")
	}

	next := fallbackIndex

	for i, uri := range conns {
		// Фрагмент ссылки (#имя) — это тег outbound'а: именно под ним узел
		// уедет в config.json и на него сошлются правила.
		tag := extractURIFragment(uri)
		unnamed := tag == ""
		if unnamed {
			next++
			tag = fmt.Sprintf("server-%d", next)
		}
		// SPEC 118 Т2: тело материализуется СРАЗУ, тем же путём, что у
		// миграции и fetch. Узел без тела собирать не из чего.
		//
		// SPEC 119: материализуется ИСХОДНИК, а не выведенная из него ссылка.
		// У вставленного конфига wg-quick это блок [Interface]…[Peer] со
		// всеми комментариями; у обычной ссылки исходник — она сама.
		material := uri
		if i < len(rawOf) && rawOf[i] != "" {
			material = rawOf[i]
		}
		mat, matErr := config.MaterializeServerNode(material, nil)
		if matErr != nil {
			debuglog.WarnLog("AddSources: URI %q not parsed: %v — node not added", uri, matErr)
			continue
		}
		res.Nodes = append(res.Nodes, corestate.Node{
			Kind:    corestate.SourceKindServer,
			Enabled: true,
			Tag:     tag,
			Body:    mat.Body,
			Origin:  &corestate.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw},
		})
		res.URIOf = append(res.URIOf, uri)
		res.Unnamed = append(res.Unnamed, unnamed)
	}

	for _, jn := range jsonNodes {
		// Имя из JSON-узла — тег outbound'а: под ним узел знают правила и
		// фильтры Направлений.
		tag := jn.Label
		unnamed := tag == ""
		if unnamed {
			next++
			tag = fmt.Sprintf("server-%d", next)
		}
		mat, matErr := config.MaterializeServerNode("", jn.ConfigJSON)
		if matErr != nil {
			debuglog.WarnLog("AddSources: JSON node %q not parsed: %v — node not added", tag, matErr)
			continue
		}
		res.Nodes = append(res.Nodes, corestate.Node{
			Kind:    corestate.SourceKindServer,
			Enabled: true,
			Tag:     tag,
			Body:    mat.Body,
			Origin:  &corestate.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw},
		})
		res.URIOf = append(res.URIOf, "")
		res.Unnamed = append(res.Unnamed, unnamed)
	}

	return res, nil
}

// silentTiming — заглушка тайминга для `classifyInputLines`.
//
// Классификатор просит интерфейс замера только ради debug-лога; ядро зовут и
// из путей, где своего таймера нет, и заводить его ради одной строки лога
// значило бы тащить `debuglog.StartTiming` в каждый вызов.
type silentTiming struct{}

func (silentTiming) LogTiming(string, time.Duration) {}
