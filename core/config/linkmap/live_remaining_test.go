package linkmap

// Страж остатка кампании SPEC 133: какие секции-мапперы ЕЩЁ НЕ ведут разбор.
//
// Атрибут `live` временный. Пока он не стоит у всех секций, в конвейере два
// пути, и этот тест держит их границу ПОИМЁННО: список ожидаемого остатка
// живёт рядом, каждое включение `live` его укорачивает, а расхождение с
// реестром красит тест.
//
// Смысл не в том, чтобы поймать опечатку. Смысл в том, чтобы остаток нельзя
// было забыть: пустой список означает «пора удалять сам атрибут, старый вход
// и эту проверку».
//
// Список — ДАННЫЕ теста, а не логика движка: греп-страж
// TestNoSchemeNamesInEngine читает только нетестовые файлы пакета, и имена
// схем здесь законны ровно потому, что это ожидание, а не поведение.

import (
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/registry"
)

// notLiveYet — секции `uri`, которые В РЕЕСТРЕ ЕСТЬ, но разбор ещё не ведут.
//
// Волна 19.09.2026 сняла отсюда trojan, vless, anytls, socks, ssh, http,
// naive, shadowsocks и vmess — все секции `uri`, которые в реестре есть,
// разбор теперь ведут.
//
// Схемы, у которых секции `uri` НЕТ ВОВСЕ (hysteria, hysteria2, masque, tuic,
// wireguard), сюда не попадают: переключать нечего, пока секция не написана.
// Их остаток держит schemesWithoutURISection ниже — иначе «секции нет» и
// «секция есть, но спит» слились бы в один молчаливый пропуск.
//
// Список ПУСТ: удалять атрибут `live` и старый вход разбора рано — сперва
// должен опустеть schemesWithoutURISection, иначе ссылки пяти схем остались
// бы без разбора вовсе.
var notLiveYet = []string{}

// schemesWithoutURISection — схемы ссылок, секции `uri` у которых ещё не
// написаны (волны W5/W6, `TASKS.md`). Список сокращается по мере написания
// секций; пустой — вся ссылочная область описана реестром.
var schemesWithoutURISection = []string{
	"wireguard",
}

func TestMappersWithoutLive(t *testing.T) {
	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}

	// Секции `uri`, которые в реестре ЕСТЬ, но разбор не ведут.
	var got []string
	for _, scheme := range set.Schemes() {
		m, ok := set.Mapper(scheme, "uri")
		if !ok {
			continue
		}
		if !m.Live {
			got = append(got, scheme)
		}
	}
	sort.Strings(got)

	// Схемы, секции `uri` которых ещё не написаны: проверяется, что их и
	// вправду нет. Иначе «написал секцию и забыл вычеркнуть» прошло бы молча.
	var missing []string
	for _, scheme := range schemesWithoutURISection {
		if _, ok := set.Mapper(scheme, "uri"); !ok {
			missing = append(missing, scheme)
		}
	}
	sort.Strings(missing)

	want := append([]string{}, notLiveYet...)
	sort.Strings(want)
	wantMissing := append([]string{}, schemesWithoutURISection...)
	sort.Strings(wantMissing)

	if strings.Join(missing, ",") != strings.Join(wantMissing, ",") {
		t.Errorf("список схем БЕЗ секции uri разошёлся с ожиданием\n"+
			" секции и вправду нет у: %v\n"+
			" ожидалось:              %v\n"+
			"Написал секцию — вычеркни схему из schemesWithoutURISection и\n"+
			"впиши в notLiveYet, пока она не прошла сверку.",
			missing, wantMissing)
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("остаток кампании разошёлся с ожиданием\n"+
			" в реестре без live: %v\n"+
			" ожидалось:          %v\n"+
			"Если схема переключена — вычеркни её из notLiveYet ЭТИМ ЖЕ коммитом.\n"+
			"Если список опустел — удали атрибут live, старый вход разбора и этот тест.",
			got, want)
	}

	if len(got) == 0 && len(missing) == 0 {
		t.Error("остаток пуст: пора снимать атрибут live и рукописный вход разбора")
	}
}
