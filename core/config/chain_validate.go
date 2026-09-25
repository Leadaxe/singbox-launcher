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
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
)

// NodeRequiresPath — тело узла ТРЕБУЕТ путь path: без него санитайзер реестра
// дописал бы его обратно (связь `requires` с `set`, контракт 1.1.61).
//
// Это вопрос каталога `strip` цепочки к звену: снимать ключ `tls.utls` у
// REALITY-узла нельзя — ядро отвергает такую цепочку отказом старта
// (`protocol/chain/transform.go:228-232`). Кто что требует, знает реестр
// (tls.reality.enabled → tls.utls.enabled), здесь имён полей и схем нет.
func NodeRequiresPath(node *ParsedNode, path string) bool {
	if node == nil || node.Outbound == nil {
		return false
	}
	return nodeflow.StripBlocked(node.Scheme, node.Outbound, path)
}

// ChainStripsKey — снимает ли цепочка ключ каталога `strip`.
//
// Учитывается и общий переключатель, и точечный патч: `strip` перекрывает
// strip_evasion в обе стороны, и смотреть только на один из них значило бы
// пропустить половину случаев.
func ChainStripsKey(c *configtypes.SourceChain, key string) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Strip[key]; ok {
		return v
	}
	def, _ := configtypes.ChainStripDefault(key)
	return c.StripEvasionEnabled() && def
}

// ChainHopsRequiring — теги позиций цепочки, чьи узлы требуют путь path.
//
// Проверяются позиции с индексом ≥ 1: strip применяется к звеньям, а
// позиция 0 идёт в сеть как есть и её опции не трогаются
// (`protocol/chain/chain.go` — звено создаётся начиная со второй позиции).
func ChainHopsRequiring(c *configtypes.SourceChain, nodesByTag map[string]*ParsedNode, path string) []string {
	if c == nil || len(nodesByTag) == 0 {
		return nil
	}
	var out []string
	hops := c.HopsOrNil()
	for i := 1; i < len(hops); i++ {
		if NodeRequiresPath(nodesByTag[hops[i]], path) {
			out = append(out, hops[i])
		}
	}
	return out
}

// ChainUnstripNote — ключ каталога `strip`, снятый с патча цепочки по
// `on_hop_required` реестра, и позиции, которые его требуют.
type ChainUnstripNote struct {
	Key  string
	Code string
	Hops []string
}

// ChainUnstripRequired — ключи каталога `strip`, которые цепочка сняла бы у
// звена, чьё тело их требует, и копия цепочки, где они не снимаются.
//
// Какие ключи так судятся и что делать, объявляет реестр (`chain.json`
// strip.fields.<ключ>.on_hop_required, контракт 1.1.61). Каталог ядро
// применяет ко всем звеньям разом, поэтому «не снимать у одного» выражается
// только патчем `ключ: false` на всю цепочку: отпечаток остаётся у всех
// звеньев, а цепочка собирается — прежде она выпадала целиком.
//
// Без находок возвращается исходная цепочка.
func ChainUnstripRequired(c *configtypes.SourceChain, nodesByTag map[string]*ParsedNode) (*configtypes.SourceChain, []ChainUnstripNote) {
	if c == nil || len(nodesByTag) == 0 {
		return c, nil
	}
	var notes []ChainUnstripNote
	for _, key := range configtypes.ChainStripKeys() {
		rule := configtypes.ChainStripOnHopRequired(key)
		if rule == nil || rule.Action != registry.HopRequiredUnstrip || !ChainStripsKey(c, key) {
			continue
		}
		if hops := ChainHopsRequiring(c, nodesByTag, key); len(hops) > 0 {
			notes = append(notes, ChainUnstripNote{Key: key, Code: rule.Code, Hops: hops})
		}
	}
	if len(notes) == 0 {
		return c, nil
	}
	out := *c
	out.Strip = make(map[string]bool, len(c.Strip)+len(notes))
	for k, v := range c.Strip {
		out.Strip[k] = v
	}
	for _, n := range notes {
		out.Strip[n.Key] = false
	}
	return &out, notes
}

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
