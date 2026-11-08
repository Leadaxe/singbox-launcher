// File origin_kind.go — вид происхождения узла по реестру видов источника.
//
// Текст происхождения приходит в одно и то же поле четырьмя диалектами:
// share-URI, блок wg-quick, JSON sing-box и JSON Xray (голое тело, документ
// или массив). Хранимый вид (`origin.kind`, контракт 1.1.165) — ДИАЛЕКТ
// текста, и ставится он один раз, когда текст приходит снаружи: вставка,
// импорт, обновление подписки, правка источника. Кто решает — движок по
// таблице `source_kinds.json` (mapper победившего вида), а не проверка
// ключа `type`/`protocol` в коде: именно такие локальные проверки и
// разъехались у LxBox (issue #209). Правило одно на всех пишущих: форму
// источника, legacy-бэкап, материализатор и миграцию старого вида `json`.
package subscription

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/config/linkmap"
)

// Имена мапперов реестра (`mapper` вида источника, source_kinds.json) и
// их отображение в вид происхождения (BACKUP.md §2).
var originKindByMapper = map[string]string{
	"singbox": OriginKindSingbox,
	"xray":    OriginKindXray,
	"uri":     OriginKindURI,
	"conf":    OriginKindWGIni,
}

// ClassifyOriginText — вид происхождения текста по реестру и имя
// победившего вида источника (`source_kind`, для выбора формы: голое тело,
// документ, массив). Вид пуст, если таблица текст не опознала (распаковщик
// не зарегистрирован, оболочка без содержимого).
func ClassifyOriginText(text string) (kind, sourceKind string) {
	plans, err := linkmap.Planes()
	if err != nil {
		return "", ""
	}
	res := linkmap.ClassifySource(plans.Mappers(), strings.TrimSpace(text), originUnwrappers)
	// Оболочка без распаковщика (контейнер `vpn://` в поле происхождения)
	// объявляет вид, но содержимого не даёт — вид у содержимого, и назвать
	// его нечем.
	if !res.Recognized || res.Kind.Mapper == nil || res.UnwrapFailed != "" {
		return "", ""
	}
	return originKindByMapper[*res.Kind.Mapper], res.Kind.SourceKind
}

// OriginKindOfText — вид происхождения для текста по реестру. Текст, которого
// таблица не опознала: JSON → `singbox` (прежнее поведение вида `json`),
// прочее → `uri` (построчная ветка реестра стоит по умолчанию, так что сюда
// попадает только оболочка без распаковщика).
//
// Годен ли текст как узел — решает материализация; вид говорит лишь, КАК
// его читать.
func OriginKindOfText(text string) string {
	if kind, _ := ClassifyOriginText(text); kind != "" {
		return kind
	}
	if IsJSONOriginText(text) {
		return OriginKindSingbox
	}
	return OriginKindURI
}

// originUnwrappers — распаковщики оболочек для классификации текста
// происхождения. base64 — тем же декодером, что у тела подписки; контейнер
// Amnezia `vpn://` в поле происхождения не живёт (его узлы хранят `.conf`,
// контракт 1.1.72), распаковщик ему здесь не нужен.
var originUnwrappers = map[string]linkmap.Unwrapper{
	"base64_utf8": func(text string) ([]string, error) {
		raw, err := DecodeSubscriptionContent([]byte(text))
		if err != nil {
			return nil, err
		}
		return []string{string(raw)}, nil
	},
}

// IsJSONOriginText — текст есть JSON-объект или массив (валидный JSON,
// начинающийся с `{` или `[`). Это проверка ФОРМЫ, не диалекта: ссылка и
// блок wg-quick так не начинаются.
func IsJSONOriginText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return false
	}
	return json.Valid([]byte(t))
}
