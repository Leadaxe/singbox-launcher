package subscription

import (
	"fmt"
	"net/url"
)

// --- SOCKS ---

func shareURIFromSocks(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" || port <= 0 {
		return "", fmt.Errorf("%w: socks needs server, server_port", ErrShareURINotSupported)
	}
	user := mapGetString(out, "username")
	pass := mapGetString(out, "password")
	var userinfo *url.Userinfo
	if user != "" || pass != "" {
		userinfo = url.UserPassword(user, pass)
	}
	// Версию протокола ссылка несёт СХЕМОЙ: у socks-ссылки параметра под неё
	// нет ни в одном диалекте. Пара с парсером обязательна — узел с version
	// 4/4a, отданный этой ссылкой, обязан вернуться собой же (round-trip
	// корпуса), а значит эмиттер и парсер читают одну таблицу.
	u := &url.URL{
		Scheme:   socksSchemeForVersion(mapGetString(out, "version")),
		User:     userinfo,
		Host:     hostPort(server, port),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}

// socksSchemeForVersion — версия протокола тела → написание схемы ссылки.
//
// У ядра тип узла один — `socks`, а версия живёт ОТДЕЛЬНЫМ полем тела
// (`version` ∈ "4" | "4a" | "5", пусто = 5; option/socks.go). В ссылке же
// параметра под версию нет ни в одном диалекте — её несёт САМА СХЕМА, ровно
// как TLS у proxy-https:// (registry/protocols/http.json).
//
// Обратное направление — разбор — таблицы в коде больше не держит: написания
// схемы и значение `version` объявляет секция socks.json (`scheme_sets`), и
// читает их движок. Здесь осталась только эмиссия.
//
// Версии 5 и пустой отвечает `socks5://`: это канон эмиссии обеих сторон,
// сложившийся до socks4, и менять его незачем — схемы `socks` и `socks5` НЕ
// сводятся друг к другу, потому что из схемы строится дефолтный тег
// (socks5-host-1080), а он входит в идентичность узла (CANON §1). Незнакомая
// версия сюда не доезжает — её снимает санитайзер по enum реестра
// (socks.body.version).
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
