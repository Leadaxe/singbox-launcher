// File tailscale_bundle.go — КАНОНИЧЕСКАЯ СВЯЗКА узла tailnet (SPEC 122 §2.5,
// NODE_SECTIONS.md §6, норма принята с LxBox 14.09.2026, их спека 437).
//
// # Что это
//
// Узел `type: tailscale` сам по себе бесполезен: ядро поднимает tailnet, но
// ни имена `*.ts.net`, ни адреса `100.64.0.0/10` в него не пойдут, пока рядом
// не встанут три записи — DNS-сервер MagicDNS, DNS-правило на суффикс и
// правило маршрута на подсети tailnet. Эти три записи и есть связка.
//
// # Почему ОДНО правило маршрута, а не два
//
// Внутри одного правила sing-box поля соединяются по ИЛИ: правило с
// `domain_suffix` И `ip_cidr` срабатывает на любом из двух признаков. Разнести
// их по двум правилам было бы не эквивалентно, а хуже: при FakeIP запрос
// приходит именем, `ip_cidr` его не матчит, и трафик до tailnet уходит мимо —
// молча, потому что имя резолвится, а маршрут выбирается не тот. Поэтому
// норма — ровно одно правило с обоими признаками.
//
// # Почему связка живёт отдельной функцией
//
// Её выдают три входа: конструктор «Add server → Tailscale», разбор голого
// узла из документа/вкладки JSON и импорт голого узла из конфига. Второй
// реализацией они разъехались бы на первой же правке нормы — тот самый класс
// расхождений, который в этом проекте уже стоил трёх схем («эмиттер и парсер
// ходят парой»). Сборка sing-box-формы — здесь, перевод её в записи
// состояния — прежней общей функцией NodeSectionsFromSingbox.
//
// # Что связка НЕ делает
//
// Она не возвращается к узлу, у которого секции сняли: снятие связки —
// осознанное действие пользователя (документ без `dns`/`route` на вкладке
// JSON), и подставлять её обратно значило бы не давать её снять вовсе.
// Поэтому подстановка живёт на путях РОЖДЕНИЯ узла, а не на пути правки.
package state

import "encoding/json"

const (
	// TailscaleCGNATRange / TailscaleCGNATRange6 — адресное пространство
	// tailnet: RFC 6598 (CGNAT) и ULA-подсеть Tailscale.
	TailscaleCGNATRange  = "100.64.0.0/10"
	TailscaleCGNATRange6 = "fd7a:115c:a1e0::/48"

	// TailscaleMagicDNSSuffix — доменный суффикс MagicDNS.
	TailscaleMagicDNSSuffix = ".ts.net"

	// TailscaleDNSTagSuffix — хвост тега DNS-сервера связки. Полный тег
	// собирается как `@{self}-dns`: на сборке `@{self}` заменяется финальным
	// тегом узла, поэтому два узла tailnet своими серверами не сталкиваются.
	TailscaleDNSTagSuffix = "dns"
)

// TailscaleDNSServerTag — тег DNS-сервера канонической связки.
func TailscaleDNSServerTag() string {
	return SelfPlaceholderBraced + "-" + TailscaleDNSTagSuffix
}

// TailscaleBundleFragments — каноническая связка в sing-box-форме.
//
// Ссылки на сам узел пишутся `@self`, тег сервера — `@{self}-dns`: связка
// обязана пережить и переименование узла, и переезд на машину, где тег
// `ts-dns` уже занят ЧУЖИМ сервером (NODE_SECTIONS.md §7).
func TailscaleBundleFragments() SingboxNodeFragments {
	dnsTag := TailscaleDNSServerTag()
	return SingboxNodeFragments{
		DNSServers: []json.RawMessage{
			mustTailscaleFragment(map[string]interface{}{
				"type":     "tailscale",
				"tag":      dnsTag,
				"endpoint": SelfPlaceholder,
			}),
		},
		DNSRules: []json.RawMessage{
			mustTailscaleFragment(map[string]interface{}{
				"domain_suffix": []interface{}{TailscaleMagicDNSSuffix},
				"server":        dnsTag,
			}),
		},
		// ОДНО правило: внутри правила поля соединяются по ИЛИ, и при FakeIP
		// один только `ip_cidr` имена бы не поймал (см. шапку файла).
		RouteRules: []json.RawMessage{
			mustTailscaleFragment(map[string]interface{}{
				"domain_suffix": []interface{}{TailscaleMagicDNSSuffix},
				"ip_cidr":       []interface{}{TailscaleCGNATRange, TailscaleCGNATRange6},
				"outbound":      SelfPlaceholder,
			}),
		},
	}
}

// TailscaleCanonicalSections — каноническая связка в ХРАНИМОЙ форме.
//
// Переводит те же фрагменты той же общей функцией, что и все прочие входы
// секций: записи связки обязаны быть неотличимы от записей, приехавших из
// вставленного конфига.
func TailscaleCanonicalSections() (*NodeSections, error) {
	return NodeSectionsFromSingbox(TailscaleBundleFragments())
}

// DefaultTailscaleSections — каноническая связка или nil, если собрать её не
// удалось.
//
// Отказ перевода означал бы, что норма этого файла разошлась с правилами
// перевода; узел тогда приезжает голым — это хуже, чем со связкой, но лучше,
// чем отказ импорта целиком.
func DefaultTailscaleSections() *NodeSections {
	out, err := TailscaleCanonicalSections()
	if err != nil || out.IsEmpty() {
		return nil
	}
	return out
}

// ApplyTailscaleDefaultSections подставляет каноническую связку ГОЛОМУ узлу
// tailnet — тому, у которого секций нет вовсе.
//
// Возвращает прежнее значение, если связка у узла уже есть: своя связка
// пользователя не трогается никогда, даже если она отличается от канонической.
//
// Признак «тело это tailnet» решает ВЫЗЫВАЮЩИЙ: таблица схем живёт в
// core/config и core/config/subscription, а этот пакет про схемы не знает.
func ApplyTailscaleDefaultSections(isTailscale bool, sections *NodeSections) *NodeSections {
	if !isTailscale || !sections.IsEmpty() {
		return sections
	}
	if out := DefaultTailscaleSections(); out != nil {
		return out
	}
	return sections
}

// mustTailscaleFragment — литерал связки в JSON. Вход — константы этого
// файла, поэтому отказ маршалинга здесь невозможен и проглатывается: пустой
// фрагмент отсеет перевод.
func mustTailscaleFragment(m map[string]interface{}) json.RawMessage {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return raw
}
