package subscription

// Схемы семейства SOCKS и их версия протокола.
//
// У ядра тип узла один — `socks`, а версия живёт ОТДЕЛЬНЫМ полем тела
// (`version` ∈ "4" | "4a" | "5", пусто = 5; option/socks.go). В ссылке же
// версии своего параметра нет ни в одном диалекте — её несёт САМА СХЕМА,
// ровно как TLS у proxy-https:// (registry/protocols/http.json). Поэтому
// схема-дискриминатор и версия тела — одна таблица, а не разбросанные
// сравнения: пара «парсер ↔ эмиттер» обязана читать её с одного места
// (память emitter-parser-pairing).
//
// Схемы `socks` и `socks5` НЕ сводятся друг к другу: из схемы строится
// дефолтный тег (socks5-host-1080), а он входит в идентичность узла
// (CANON §1, комментарий у ветки socks5:// в node_parser_core.go).

// socksVersionByScheme — схема ссылки → значение `version` в теле.
//
// У socks:// и socks5:// это "5" ЯВНО, хотя ядру хватило бы и отсутствия
// ключа: так пишут обе стороны с самого начала, значение стоит в ожиданиях
// корпуса (contract/corpus/uri/socks/*) и в телах всех живых socks-узлов.
// Снять его значило бы переписать тело каждому существующему узлу и всю
// socks-часть корпуса ради нуля разницы для ядра — цена без выгоды. Правило
// же «дефолт ядра не материализуем» (CANON §2.4) остаётся в силе там, где
// решает реестр: тело в форме sing-box с явным "5" проходит как есть, тело
// без ключа ключа не получает.
var socksVersionByScheme = map[string]string{
	"socks":   "5",
	"socks5":  "5",
	"socks4":  "4",
	"socks4a": "4a",
}

// isSocksScheme — принадлежит ли схема семейству SOCKS.
func isSocksScheme(scheme string) bool {
	_, ok := socksVersionByScheme[scheme]
	return ok
}

// IsSocksScheme — тот же вопрос наружу: пакет config собирает тело узла по
// схеме и обязан узнавать ВСЕ четыре её написания. Второй таблицы там
// заводить нельзя — она разъехалась бы с этой (та же причина, что у
// SchemeFromSingboxType).
func IsSocksScheme(scheme string) bool {
	return isSocksScheme(scheme)
}

// socksVersionForScheme — версия протокола, которую диктует схема ссылки.
func socksVersionForScheme(scheme string) string {
	return socksVersionByScheme[scheme]
}

// socksSchemeForVersion — обратный перевод для эмиссии share-URI.
//
// Версии 5 и пустой отвечает `socks5://`: это канон эмиссии обеих сторон,
// сложившийся до socks4, и менять его незачем. Незнакомая версия сюда не
// доезжает — её снимает санитайзер по enum реестра (socks.body.version).
func socksSchemeForVersion(version string) string {
	switch version {
	case "4":
		return "socks4"
	case "4a":
		return "socks4a"
	default:
		return "socks5"
	}
}
