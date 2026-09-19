package subscription

// Приватный ключ в share-URI: один предикат на все точки выдачи ссылки.
//
// Решение владельца 18.09.2026 («как в LxBox»): ссылка, которая несёт
// ПРИВАТНЫЙ КЛЮЧ, отдаётся только после явного подтверждения. Пароли и uuid
// сюда НЕ входят — их поведение не меняется.
//
// Судим по ТЕЛУ узла и схеме, а не по тексту готовой ссылки: grep по "?
// private_key=" совпал бы и на private_key_path (путь к файлу — ключа в
// ссылке нет), и промахнулся бы мимо wireguard/masque, где ключ лежит в
// userinfo без имени параметра.

import "strings"

// shareURISecretCarrierFields — поля тела, чьё НЕПУСТОЕ значение уезжает в
// share-URI как приватный ключ. Ключ карты — sing-box type узла.
//
// Пара с эмиттерами: shareuri_ssh.go (?private_key=),
// shareuri_wireguard.go и shareuri_masque.go (userinfo). Новая схема с
// приватным ключом добавляется сюда в тот же заход, что и её эмиссионная
// ветка, иначе ссылка утечёт молча.
var shareURISecretCarrierFields = map[string][]string{
	"ssh":       {"private_key"},
	"wireguard": {"private_key"},
	"masque":    {"private_key"},
}

// ShareURICarriesPrivateKey сообщает, будет ли share-URI этого узла нести
// приватный ключ.
//
// out — тело узла из config.json (outbounds[] либо endpoints[]), в той же
// форме, что принимает ShareURIFromOutbound.
//
// private_key_path не считается: в ссылку уезжает ПУТЬ к файлу, а не сам
// ключ. private_key_passphrase тоже не ключ — без ключа она бесполезна.
func ShareURICarriesPrivateKey(out map[string]interface{}) bool {
	if out == nil {
		return false
	}
	typ := strings.ToLower(strings.TrimSpace(mapGetString(out, "type")))
	fields, ok := shareURISecretCarrierFields[typ]
	if !ok {
		return false
	}
	for _, f := range fields {
		if shareURISecretFieldFilled(out[f]) {
			return true
		}
	}
	return false
}

// ShareURITextCarriesPrivateKey — тот же вопрос там, где тела узла на руках
// НЕТ, а есть только строка ссылки: поле «Server URI» после ручной правки,
// исходник неразобранной строки превью (origin.raw).
//
// Разбираем ссылку тем же парсером, что и при импорте, и спрашиваем предикат
// по ПОЛУЧЕННОМУ телу — грепа по тексту здесь нет намеренно: "private_key="
// совпало бы и на private_key_path, и промахнулось бы мимо
// wireguard/masque, где ключ лежит в userinfo без имени параметра.
//
// Ссылка не разбирается (мусор, чужой формат, пустая строка) — false:
// диалога нет, копируем как есть. Тела у такой строки не существует, значит
// и утверждать про ключ в ней нечего; а держать человека диалогом над каждой
// нечитаемой строкой значит приучить жать «Да» не читая.
func ShareURITextCarriesPrivateKey(uri string) bool {
	raw := strings.TrimSpace(uri)
	if raw == "" {
		return false
	}
	node, err := ParseNode(raw, nil)
	if err != nil || node == nil {
		return false
	}
	return ShareURICarriesPrivateKey(node.Outbound)
}

// shareURISecretFieldFilled — непустое ли значение секретного поля. Форма
// listable_string (ssh private_key: строка ЛИБО массив строк) и JSON-мусор
// разбираются здесь, а не у каждого вызывающего.
func shareURISecretFieldFilled(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(t) != ""
	case []string:
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
		return false
	case []interface{}:
		for _, e := range t {
			if shareURISecretFieldFilled(e) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
