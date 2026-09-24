package linkmap

// Раннер уровня ВИДА ИСТОЧНИКА: «текст → вид → число элементов»
// (SPEC 133 §3A, таблица contract/registry/source_kinds.json).
//
// Отличается от остальных раннеров семьи тем, ЧТО именно сверяет.
// TestEngineVsFixtures/TestEngineVsXrayCorpus/TestEngineVsConfCorpus берут
// уже нарезанный элемент и сверяют СОБРАННОЕ ТЕЛО узла; здесь проверяется
// ступень выше и до них: чем текст подписки оказался целиком и на сколько
// элементов он распался. Ошибка этой ступени тел не портит — она их не даёт
// вовсе (подписка на 500 узлов молча превращается в ноль), и потому её
// не видит ни один из трёх.
//
// Кейсы записаны ТАБЛИЦЕЙ, а не файлами корпуса: проверяется классификация,
// и телу узла здесь взяться неоткуда — тела уже стережёт корпус. Каждый кейс
// — минимальный текст, на котором ветка отличается от соседней, плюс
// ожидаемое число элементов: именно им ловится нарезка, промахнувшаяся
// уровнем (элементом стал конфиг целиком вместо каждого его outbound'а).
//
// Запуск:
//
//	go test ./core/config/linkmap -run TestSourceKinds
//	go test ./core/config/linkmap -run TestSourceKinds -v

import (
	"encoding/base64"
	"fmt"
	"testing"

	"singbox-launcher/core/config/registry"
)

// TestSourceKinds — таблица «текст → вид → число элементов».
func TestSourceKinds(t *testing.T) {
	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}
	if set.SourceKinds() == nil {
		t.Fatal("source_kinds.json не загрузился — проверь имя файла и ключ верхнего уровня")
	}

	// Распаковщики передаются ИЗВНЕ: формат контейнера предикатом не
	// выражается, но зовётся по объявленному имени, а не веткой в сниффере.
	// В раннере они игрушечные — проверяется вызов по имени и повторный
	// детект, а не сами кодеки (их стережёт корпус тел).
	unwrappers := map[string]Unwrapper{
		"base64_utf8": func(text string) ([]string, error) {
			raw, err := base64.StdEncoding.DecodeString(text)
			if err != nil {
				raw, err = base64.RawURLEncoding.DecodeString(text)
				if err != nil {
					return nil, err
				}
			}
			return []string{string(raw)}, nil
		},
	}

	cases := []struct {
		name string
		text string
		// kind — ожидаемый вид источника.
		kind string
		// elements — сколько элементов дала нарезка.
		elements int
		// skipped — сколько строк пропущено (пустые в счёт не идут).
		skipped int
		// unwrapped — сколько оболочек снято.
		unwrapped int
		// unwrapFailed — имя распаковщика, на котором вид отвергнут. Пусто
		// у всех кейсов, кроме объявленного отказа (исчерпан предел
		// глубины): отказ обязан быть ОЖИДАЕМЫМ, иначе он неотличим от
		// сломавшейся распаковки.
		unwrapFailed string
		// byDefault — вид выигран веткой «всё остальное». Поле проверяется
		// ЯВНО только там, где победитель определился ПОВТОРНЫМ судом
		// (отвергнутая обёртка): Matched/ByDefault обязаны описывать его, а
		// не отвергнутый вид, иначе лог называет одно, а имя вида — другое.
		byDefault bool
		// ambiguous — под предикат подходит НЕ ОДНА ветка, и это объявлено
		// таблицей: пары sing-box/Xray различаются признаком диалекта, а
		// неоднозначный вход (есть оба признака либо нет ни одного) решает
		// меньший priority. Остальные кейсы обязаны совпасть ровно один раз.
		ambiguous bool
	}{
		{
			name:     "список ссылок — ветка по умолчанию",
			text:     "vless://u@a.com:443#a\nvmess://u@b.com:443#b\n",
			kind:     "uri_lines",
			elements: 2,
		},
		{
			name:     "комментарии и пустые строки не элементы",
			text:     "# заголовок\nvless://u@a.com:443#a\n\n// прочь\n;прочь\nss://x@b.com:8388#b\n",
			kind:     "uri_lines",
			elements: 2,
			skipped:  3,
		},
		{
			name:      "base64 целиком — распаковка и повторный детект",
			text:      base64.StdEncoding.EncodeToString([]byte("vless://u@a.com:443#a\nvless://u@b.com:443#b\n")),
			kind:      "uri_lines",
			elements:  2,
			unwrapped: 1,
		},
		{
			// Предел max_unwrap_depth только тогда что-то ограничивает,
			// когда до него вообще можно дойти: промежуточный слой двойной
			// обёртки — чистый алфавит base64, и requires_after_unwrap на
			// нём проваливается. Отличать «обёртка не кончилась» от
			// «обёртки не было» обязан повторный детект, иначе двойная
			// подписка даёт ОДИН элемент-блоб и ноль узлов.
			name:      "base64 внутри base64 — распаковка до документа",
			text:      base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString([]byte("vless://u@a.com:443#a\nvless://u@b.com:443#b\n")))),
			kind:      "uri_lines",
			elements:  2,
			unwrapped: 2,
		},
		{
			// Предел исчерпан: вид объявлен (человеку важно знать, чем текст
			// оказался), элементов нет, отказ тихий — ни паники, ни ухода в
			// рекурсию.
			name:         "тройная обёртка — предел глубины, отказ без паники",
			text:         base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString([]byte("vless://u@a.com:443#a\nvless://u@b.com:443#b\n")))))),
			kind:         "base64_wrapped",
			elements:     0,
			unwrapped:    2,
			unwrapFailed: "base64_utf8",
		},
		{
			// Страховка requires_after_unwrap: строка ссылок без спецсимволов
			// проходит алфавит base64, и без проверки правдоподобия
			// распакованный мусор вытеснил бы настоящий список.
			name:      "текст из алфавита base64, но не обёртка",
			text:      "trojanpasswordlookalike\nanotherlonglineofletters\n",
			kind:      "uri_lines",
			elements:  2,
			byDefault: true,
		},
		{
			// BOM — мусор кодировки файла, а не признак формата: невидимый
			// U+FEFF сдвигает первый значащий символ, и документ перестаёт
			// быть JSON'ом сразу для ВСЕХ предикатов (уезжал в построчную
			// ветку одним элементом-блобом).
			name:     "BOM перед JSON не меняет вид документа",
			text:     "\uFEFF" + `{"log":{},"outbounds":[{"type":"vless"},{"type":"direct"}]}`,
			kind:     "singbox_config",
			elements: 2,
		},
		{
			name:     "BOM перед .conf не меняет вид документа",
			text:     "\uFEFF" + "[Interface]\nPrivateKey = aaa\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = bbb\n",
			kind:     "wireguard_conf",
			elements: 1,
		},
		{
			name:     "одиночный sing-box outbound",
			text:     `{"type":"vless","server":"a.com","server_port":443}`,
			kind:     "singbox_outbound",
			elements: 1,
		},
		{
			// Одиночный selector несёт и type, и outbounds: он outbound,
			// а не конфиг — решает меньший priority, а не порядок строк.
			name:     "selector — outbound, а не конфиг",
			text:     `{"type":"selector","outbounds":["a","b"]}`,
			kind:     "singbox_outbound",
			elements: 1,
		},
		{
			name:     "полный sing-box конфиг — элементы это его outbounds",
			text:     `{"log":{},"outbounds":[{"type":"vless"},{"type":"direct"}]}`,
			kind:     "singbox_config",
			elements: 2,
		},
		{
			// endpoints равноправен outbounds: конфиг может состоять
			// из одних WireGuard-узлов.
			name:     "конфиг из одних endpoints",
			text:     `{"endpoints":[{"type":"wireguard"}]}`,
			kind:     "singbox_config",
			elements: 1,
		},
		{
			name:     "outbounds и endpoints складываются",
			text:     `{"outbounds":[{"type":"vless"}],"endpoints":[{"type":"wireguard"}]}`,
			kind:     "singbox_config",
			elements: 2,
		},
		{
			name:     "массив голых sing-box outbound'ов",
			text:     `[{"type":"vless"},{"type":"trojan"}]`,
			kind:     "singbox_outbound_array",
			elements: 2,
		},
		{
			name:     "массив sing-box конфигов — элемент это outbound, не конфиг",
			text:     `[{"outbounds":[{"type":"vless"},{"type":"direct"}]},{"outbounds":[{"type":"trojan"}]}]`,
			kind:     "singbox_config_array",
			elements: 3,
			// Ветка xray_config_array ловит `[].outbounds` без проверки
			// диалекта и потому подходит тоже; разводит их priority
			// 30 < 31 — так решал и рукописный код.
			ambiguous: true,
		},
		{
			// Признак диалекта — ключ protocol у ЭЛЕМЕНТА: без него полный
			// Xray-конфиг забрала бы ветка sing-box-конфига, и его элементы
			// уехали бы в маппер, который их не читает (узлов ноль).
			name:     "полный Xray-конфиг — диалект назвался protocol",
			text:     `{"inbounds":[],"outbounds":[{"protocol":"vless"},{"protocol":"freedom"}]}`,
			kind:     "xray_config",
			elements: 2,
			// singbox_config ловит `outbounds` у объекта без проверки
			// диалекта; разводит их priority 35 < 50.
			ambiguous: true,
		},
		{
			name:     "массив Xray-конфигов",
			text:     `[{"outbounds":[{"protocol":"vless"}]},{"outbounds":[{"protocol":"trojan"}]}]`,
			kind:     "xray_config_array",
			elements: 2,
		},
		{
			// Ветка НОВАЯ: сегодня такой массив доезжает до построчного
			// разбора и даёт ноль узлов (DELTAS D133-37).
			name:     "массив голых Xray-outbound'ов",
			text:     `[{"protocol":"vless"},{"protocol":"trojan"}]`,
			kind:     "xray_outbound_array",
			elements: 2,
		},
		{
			name:     "одиночный Xray-outbound",
			text:     `{"protocol":"vless","settings":{}}`,
			kind:     "xray_outbound",
			elements: 1,
		},
		{
			name:     "wg-quick .conf",
			text:     "[Interface]\nPrivateKey = aaa\nAddress = 10.0.0.2/32\n\n[Peer]\nPublicKey = bbb\n",
			kind:     "wireguard_conf",
			elements: 1,
		},
		{
			// Комментарий над [Interface] законен и несёт имя узла:
			// судится ПЕРВАЯ не-комментарная секция, а не наличие секции.
			name:     "комментарий над [Interface] не мешает",
			text:     "# Узел Франкфурт\n[Interface]\nPrivateKey = aaa\n",
			kind:     "wireguard_conf",
			elements: 1,
		},
		{
			// Заготовка без пира — законный .conf: требование [Peer]
			// уронило бы её в построчную ветку (GRAMMAR_SYNC §4 №10).
			name:     "заготовка .conf без [Peer]",
			text:     "[Interface]\nPrivateKey = aaa\nAddress = 10.0.0.2/32\n",
			kind:     "wireguard_conf",
			elements: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := ClassifySource(set, tc.text, unwrappers)
			if !res.Recognized {
				t.Fatalf("вид не опознан вовсе (Matched=%v)", res.Matched)
			}
			if res.Kind.SourceKind != tc.kind {
				t.Errorf("вид %q, ожидался %q", res.Kind.SourceKind, tc.kind)
			}
			if got := len(res.Elements); got != tc.elements {
				t.Errorf("элементов %d, ожидалось %d", got, tc.elements)
			}
			if res.SkippedLines != tc.skipped {
				t.Errorf("пропущено строк %d, ожидалось %d", res.SkippedLines, tc.skipped)
			}
			if res.UnwrapDepth != tc.unwrapped {
				t.Errorf("снято оболочек %d, ожидалось %d", res.UnwrapDepth, tc.unwrapped)
			}
			if res.UnwrapFailed != tc.unwrapFailed {
				t.Errorf("отказ распаковки %q, ожидался %q", res.UnwrapFailed, tc.unwrapFailed)
			}
			// Совпасть обязан РОВНО один вид — кроме объявленных пар
			// sing-box/Xray, где неоднозначность разводит priority.
			// Незаявленное перекрытие значит, что поведение держится на
			// номере, а не на предикате, и переезд ветки его сломает.
			if tc.byDefault {
				if !res.ByDefault {
					t.Errorf("вид %q выигран не default-веткой, а Matched=%v", res.Kind.SourceKind, res.Matched)
				}
				if len(res.Matched) != 0 {
					t.Errorf("Matched=%v у победителя default-ветки — это след отвергнутого вида", res.Matched)
				}
			}
			if !res.ByDefault && !tc.ambiguous && len(res.Matched) > 1 {
				t.Errorf("сработало несколько видов: %v — незаявленное перекрытие предикатов", res.Matched)
			}
			if tc.ambiguous && len(res.Matched) < 2 {
				t.Errorf("кейс помечен неоднозначным, но сработал один вид %v — пометку пора снять", res.Matched)
			}
		})
	}
}

// TestSourceKindsTableWellFormed — форма самой таблицы, без текстов.
//
// Стережёт то, что на кейсах не видно: уникальность priority (иначе порядок
// зависел бы от порядка строк в файле), единственность default-ветки и то,
// что у каждого вида есть чем разобрать элемент.
func TestSourceKindsTableWellFormed(t *testing.T) {
	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}
	kinds := set.SourceKindsByPriority()
	if len(kinds) == 0 {
		t.Fatal("таблица видов источника пуста")
	}

	seenPriority := make(map[int]string, len(kinds))
	seenName := make(map[string]bool, len(kinds))
	defaults := 0
	for _, k := range kinds {
		if k.SourceKind == "" {
			t.Error("вид без имени")
			continue
		}
		if seenName[k.SourceKind] {
			t.Errorf("%s: имя вида повторяется", k.SourceKind)
		}
		seenName[k.SourceKind] = true

		if prev, ok := seenPriority[k.Priority]; ok {
			t.Errorf("%s и %s: одинаковый priority %d — порядок стал бы случайным",
				prev, k.SourceKind, k.Priority)
		}
		seenPriority[k.Priority] = k.SourceKind

		if k.Detect == nil || k.Detect.IsZero() {
			t.Errorf("%s: пустой detect — такую ветку выбрать нельзя", k.SourceKind)
		}
		if k.Detect != nil && k.Detect.Default {
			defaults++
		}
		// Вид обязан либо давать элементы, либо снимать оболочку:
		// иначе текст, доехавший до него, останется без разбора.
		if k.Elements == "" && k.Unwrap == "" {
			t.Errorf("%s: нет ни elements, ни unwrap — текст останется без разбора", k.SourceKind)
		}
		// Маппер нужен там, где вид ДАЁТ элементы: кто их прочтёт, обязано
		// назвать данными. У чистой обёртки маппера нет — он определится
		// у вида, найденного после распаковки.
		if k.Elements != "" && (k.Mapper == nil || *k.Mapper == "") {
			t.Errorf("%s: даёт элементы, но не называет маппера", k.SourceKind)
		}
		if k.Redetect && k.Unwrap == "" {
			t.Errorf("%s: redetect без unwrap — пересудить нечего", k.SourceKind)
		}
	}

	if defaults != 1 {
		t.Errorf("веток default: %d, должна быть ровно одна", defaults)
	}
	// Ветка по умолчанию обязана стоять ПОСЛЕДНЕЙ в порядке проверки:
	// объявленный default с малым priority не конкурирует с настоящим
	// предикатом, но читающего человека сбивает.
	if last := kinds[len(kinds)-1]; last.Detect == nil || !last.Detect.Default {
		t.Errorf("последний по priority вид — %s, а не ветка default", last.SourceKind)
	}

	// Секции-мапперы и их формы — тот же IsZero, что у видов источника.
	// Пустой detect здесь — не «форма без условия» (та пишется без ключа
	// detect вовсе), а словарь без единого предиката: `json: {}` явный или
	// оставшийся после того, как загрузчик молча выбросил поле с именем не
	// из грамматики (`has_key` вместо `required_keys`). Движок на таком
	// словаре верен на любом элементе, формы пробуются по порядку — первая
	// такая забирает и элементы, ради которых написаны следующие. Схема
	// (TestContractMapperSectionsMatchSchema) `json: {}` пропускает, так что
	// рубеж только здесь.
	sections, forms := 0, 0
	for _, scheme := range set.Schemes() {
		for _, kind := range set.Kinds(scheme) {
			m, ok := set.Mapper(scheme, kind)
			if !ok || m == nil {
				continue
			}
			sections++
			where := fmt.Sprintf("%s mappers.%s", scheme, kind)
			checkDetectTree(t, where+".detect", m.Detect)
			for i := range m.Forms {
				forms++
				checkDetectTree(t, fmt.Sprintf("%s.forms[%d](%s).detect", where, i, m.Forms[i].ID),
					m.Forms[i].Detect)
			}
		}
	}
	if sections == 0 {
		t.Error("секций-мапперов не найдено — обход смотрит не туда")
	}
	t.Logf("проверено секций-мапперов: %d, форм: %d", sections, forms)
}

// checkDetectTree — ни один узел дерева предиката не пуст.
//
// nil на входе законен: форма без detect берёт всё (так пишется единственная
// форма секции), секция без detect по содержимому не выбирается вовсе. Обход
// вложенный: `{}` внутри all/any движок тоже считает истиной, внутри not —
// вечной ложью, и ветка молча становится «всё» либо «ничего».
func checkDetectTree(t *testing.T, where string, d *registry.Detect) {
	t.Helper()
	if d == nil {
		return
	}
	if d.IsZero() {
		t.Errorf("%s: пустой предикат — движок верен на любом элементе, запись заберёт чужие", where)
		return
	}
	checkDetectTree(t, where+".not", d.Not)
	for i := range d.All {
		checkDetectTree(t, fmt.Sprintf("%s.all[%d]", where, i), &d.All[i])
	}
	for i := range d.Any {
		checkDetectTree(t, fmt.Sprintf("%s.any[%d]", where, i), &d.Any[i])
	}
}

// TestDetectIsZeroEmptyDictionaries — пустой словарь json / ini / text
// предикатом не является: опора линтера выше.
//
// Записи синтетические и идут через тот же json.Unmarshal, что у загрузчика:
// поле с именем не из грамматики он выбрасывает молча, и `has_key` оставляет
// от предиката ровно тот же пустой словарь, что явное `{}`. Рантайм
// (Matches → matchJSON) на пустом словаре верен на любом элементе с обеих
// сторон контракта и не меняется — судит только линтер данных.
func TestDetectIsZeroEmptyDictionaries(t *testing.T) {
	cases := []struct {
		raw  string
		zero bool
	}{
		{`{"json": {}}`, true},
		{`{"json": {"has_key": ["peers"]}}`, true},
		{`{"json": {"required_keys": ["peers"]}}`, false},
		{`{"json": {"value_in": {"type": ["a", "b"]}}}`, false},
		{`{"ini": {}}`, true},
		{`{"ini": {"keys_any": ["jc"]}}`, false},
		{`{"text": {}}`, true},
		{`{"text": {"prefix_trim": "{"}}`, false},
		// Пустой словарь рядом с настоящим предикатом запись не обнуляет.
		{`{"json": {}, "scheme_in": ["x"]}`, false},
		{`{"json": {}, "default": true}`, false},
	}
	for _, c := range cases {
		if got := mustDetect(t, c.raw).IsZero(); got != c.zero {
			t.Errorf("%s: IsZero = %v, ожидалось %v", c.raw, got, c.zero)
		}
	}
}
