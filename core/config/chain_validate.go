// File chain_validate.go — предупреждения о цепочке, которые нельзя вывести
// из неё одной (SPEC 110, фаза 4).
//
// От ChainEmitError отличаются тем, что смотрят НЕ на цепочку, а на то, из
// чего она составлена: какие узлы стоят на позициях и что у них внутри.
// Эмиттер этого не знает и знать не должен — он получает уже отобранный
// набор тегов.
package config

import (
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// ChainNestedConflict — теги вложенных цепочек, стоящих не на позиции 0.
//
// Ядро допускает вложенную цепочку только первой позицией
// (`protocol/chain/chain.go:279`): звено — это «узел через предыдущую
// позицию», а цепочка не узел, её нельзя пересобрать под чужой диалер.
func ChainNestedConflict(c *configtypes.SourceChain, chainTags map[string]bool) []string {
	if c == nil || len(chainTags) == 0 {
		return nil
	}
	var out []string
	hops := c.HopsOrNil()
	for i := 1; i < len(hops); i++ {
		if chainTags[hops[i]] {
			out = append(out, hops[i])
		}
	}
	return out
}

// ChainLayerTag — служебный тег префикса цепочки: путь от клиента до позиции
// pos включительно.
//
// Парен ChainInternalTag: там эти теги распознаются, чтобы не пустить их в
// конфиг, здесь — собираются, чтобы спросить у ядра задержку. Схема имени
// принадлежит ядру (`protocol/chain`), и держать её в одном файле с
// распознавателем обязательно: разойдись они — проба молча мерила бы не то.
func ChainLayerTag(chainTag string, pos int) string {
	return chainTag + "#" + strconv.Itoa(pos)
}

// ChainInternalTag — тег вида `<chain>#<i>`, который цепочка резервирует под
// свои звенья.
//
// Такие теги существуют только в рантайме ядра: в конфиге их нет, целью
// правила они быть не могут, и предлагать их пользователю — значит дать
// собрать конфиг со ссылкой в никуда (SPEC 110 T6).
//
// Пробел перед `#` исключает совпадение: публичные подписки сплошь и рядом
// называют узлы «Germany #1», и без этой оговорки живой узел пропадал бы из
// кандидатов позиций и целей правил. Имя цепочки обрезается по краям при
// сохранении, поэтому настоящий служебный тег пробела перед `#` не несёт.
func ChainInternalTag(tag string) bool {
	i := strings.LastIndex(tag, "#")
	if i <= 0 || i == len(tag)-1 {
		return false
	}
	if tag[i-1] == ' ' || tag[i-1] == '\t' {
		return false
	}
	for _, r := range tag[i+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ChainBuiltinHopTags — служебные теги шаблона, которые форма предлагает
// позициями цепочки и которые ResolveChainSources обязан считать известными:
// шаблонные константы подмешиваются в конфиг только на финальной сборке, и
// без этого списка форма и сборка расходились бы («direct-out» предложен —
// «direct-out» не найден).
var ChainBuiltinHopTags = []string{"direct-out"}
