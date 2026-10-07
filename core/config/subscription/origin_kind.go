// File origin_kind.go — вид происхождения узла по ФОРМЕ текста.
//
// Текст происхождения приходит в одно и то же поле тремя формами: share-URI,
// блок wg-quick и JSON (тело outbound/endpoint, документ узла или массив
// тел). Хранимый вид (`origin.kind`) обязан отвечать форме текста, иначе
// Regen разбирает JSON как ссылку и падает на «no scheme» — ровно так и
// ломался узел Tailscale, чьё происхождение правили руками в поле Origin.
// Правило одно на всех пишущих: форму источника, legacy-бэкап и
// материализатор.
package subscription

import (
	"encoding/json"
	"strings"
)

// OriginKindOfText — вид происхождения для текста: JSON-объект или массив →
// `json`, блок wg-quick → `wg_ini`, всё остальное — `uri`.
//
// JSON узнаётся по форме, а не по успеху разбора узла: вид говорит, КАК
// текст читать, а годен ли он как узел — решает материализация.
func OriginKindOfText(text string) string {
	if IsJSONOriginText(text) {
		return OriginKindJSON
	}
	if len(WGConfBlocksOf(text)) > 0 {
		return OriginKindWGIni
	}
	return OriginKindURI
}

// IsJSONOriginText — текст есть JSON-объект или массив (валидный JSON,
// начинающийся с `{` или `[`). Ссылка и блок wg-quick так не начинаются.
func IsJSONOriginText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return false
	}
	return json.Valid([]byte(t))
}
