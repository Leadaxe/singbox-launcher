// File origin_label.go — восстановление подписи узла из его происхождения
// (SPEC 118 W4).
//
// Канон v7 хранит у узла сырой ТЕГ (идентичность в контейнере) и `origin.raw`
// — строку, из которой узел родился. Отображаемой подписи (`Label`) и
// комментария в каноне нет: они производные от той же строки, и второе
// хранилище имени разъезжалось бы с первым (та же ловушка, что «тег vs
// подпись» SPEC 112).
//
// Но эмиссии они нужны: переменные тег-политики `{$label}` / `{$comment}` и
// фильтры Направлений по ключам `label` / `fragment` / `comment` читают
// именно их. Поэтому подпись ВОССТАНАВЛИВАЕТСЯ здесь — теми же шагами, что
// делал парсер URI (fragment → PathUnescape → UTF-8 → sanitize → normalize),
// без разбора тела и без построения узла.
package subscription

import (
	"encoding/json"
	"net/url"
	"strings"

	"singbox-launcher/internal/textnorm"
)

// LabelFromOriginURI — подпись узла из его исходной URI-строки.
//
// Пустая строка = происхождения нет (JSON-тело, синтезированная группа,
// wg-профиль) либо во фрагменте ничего не было: вызывающий подставит сырой
// тег — он и есть лучшее известное имя.
func LabelFromOriginURI(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return ""
	}
	label := parsed.Fragment
	if label != "" {
		if decoded, derr := url.PathUnescape(label); derr == nil {
			label = decoded
		}
		fixed, valid := validateAndFixUTF8(label)
		if !valid {
			return ""
		}
		label = fixed
	}
	// Тот же фолбэк на путь, что у парсера: из userinfo имя не берётся
	// никогда (там секрет).
	if label == "" && parsed.Path != "" && parsed.Path != "/" {
		label = strings.TrimPrefix(parsed.Path, "/")
	}
	label = sanitizeForDisplay(label)
	return textnorm.NormalizeProxyDisplay(label)
}

// nameFieldsInJSONRecord — поля JSON-записи, которые несут ИМЯ.
//
// Порядок — от самого специфичного к самому общему: `tag` (sing-box и Xray),
// `ps` (vmess-JSON), `remarks` (Xray-профиль и clash-подобные тела), `name`
// (общий фолбэк). Первое непустое выигрывает: у одной записи их обычно ровно
// одно, а когда их два, специфичное и есть то, под которым запись показана.
var nameFieldsInJSONRecord = []string{"tag", "ps", "remarks", "name"}

// NameFromOriginJSON — имя записи JSON-тела из её исходника (SPEC 116 W13).
//
// Тот же вопрос, на который у URI-записи отвечает LabelFromOriginURI: «как эту
// запись зовут у провайдера». У JSON-элемента имя лежит полем, а не
// фрагментом, поэтому разбор свой — но правило одно: имя берётся ИЗ ЗАПИСИ, а
// позиционный `unsupported-N` остаётся только для записи, у которой имени нет
// вовсе.
//
// Пустая строка = исходник не JSON-объект либо ни одного именующего поля в
// нём нет.
func NameFromOriginJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return ""
	}
	for _, key := range nameFieldsInJSONRecord {
		s, ok := obj[key].(string)
		if !ok {
			continue
		}
		s = textnorm.NormalizeProxyDisplay(sanitizeForDisplay(strings.TrimSpace(s)))
		if s != "" {
			return s
		}
	}
	return ""
}

// CommentFromLabel — комментарий узла по его подписи: часть после «|», а без
// разделителя — вся подпись целиком (правило extractTagAndComment).
func CommentFromLabel(label string) string {
	_, comment := extractTagAndComment(label)
	return comment
}

// ProviderTagFromLabel — ПРОВАЙДЕРСКИЙ тег узла по его подписи: тот самый
// вход тег-политики, каким его видела старая машина (extractTagAndComment +
// normalizeFlagTag), ДО уникализации внутри тела.
//
// Разница важна и на неё опираются эталоны (SPEC 118, риск Р1): подписка с
// двумя узлами «NL-1» давала конфиговые теги `[P] NL-1 •` и `[P] NL-1 •-2` —
// уникализировался ФИНАЛЬНЫЙ тег, а не вход политики. Канон же хранит сырые
// теги уже уникализированными (`NL-1`, `NL-1-2`) — это идентичность в
// контейнере, merge-ключ. Подставь их в политику, и вышло бы
// `[P] NL-1-2 •`: другое имя, протухший выбор в кэше ядра и расхождение с
// прежним конфигом на ровном месте.
func ProviderTagFromLabel(label string) string {
	tag, _ := extractTagAndComment(label)
	return normalizeFlagTag(tag)
}
