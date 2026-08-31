// File relay_materialize.go — служебные узлы записи (релеи BYPASS) как
// самостоятельные узлы состава (SPEC 120).
//
// # Что за релеи
//
// Провайдеры отдают конфиги, где путь к серверу идёт не напрямую, а через
// промежуточный socks5-релей: у Xray это `streamSettings.sockopt.dialerProxy`,
// ссылка на другой outbound того же конфига. Так живут «BYPASS»-конфиги
// Liberty — прямой путь к их серверам зарезан, а к релею нет.
//
// Парсер это уже понимает и складывает релеи в `ParsedNode.Chain`. Но до
// состояния они не доезжали: тело эмитится без них, а сами хопы разворачивал
// только генератор конфига. Итог — релей работал, но человеку не существовал:
// ни выключить, ни выбрать, ни сослаться; каждый fetch собирал его заново.
//
// # Почему Detour, а не Hops
//
// Hops — это ЦЕПОЧКА (kind=chain, несколько позиций подряд). У релея роль
// другая и одна: «дозвониться через». Ровно это и означает Detour, и второе
// поле для того же смысла развело бы одну связь по двум местам.
//
// Многохоповый случай (релей через релей) складывается тем же полем: каждый
// звонит через следующего, последний идёт напрямую.
//
// # Имена
//
// У релея нет своего имени в записи — он там безымянный довесок, и это же
// служит признаком служебности. Имя строится от владельца: `<тег> · relay`,
// при нескольких — с номером. Уникальность обеспечивает вызывающий (теги
// внутри подписки уникализирует общая машина), здесь только форма.
package config

import (
	"fmt"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// relayTagSuffix — как зовут служебный узел, выделенный из чужой записи.
//
// Пробелы вокруг «·» намеренные: тег видит человек в списке и в правилах, и
// слитное `tag·relay` читалось бы как часть имени сервера. Шестерёнку в тег
// НЕ пишем — тег адресует узел в конфиге и в ссылках, украшать его нельзя
// (решение обкатки).
const relayTagSuffix = " · relay"

// relayNodesFromEntry — служебные узлы записи и ссылка владельца на первый.
//
// Возвращает (узлы, detour владельца). Пустой результат = у записи релеев
// нет, владельца трогать не надо.
func relayNodesFromEntry(
	subID string,
	e *subscription.ParsedBodyEntry,
	ownerTag string,
) ([]state.Node, *state.NodeLink) {
	if e == nil || e.Node == nil || len(e.Node.Chain) == 0 {
		return nil, nil
	}

	out := make([]state.Node, 0, len(e.Node.Chain))
	for i, hop := range e.Node.Chain {
		if hop == nil {
			continue
		}
		// Группа релеем быть не может: тела у неё нет, дозваниваться через
		// неё нечем. Сегодня в Chain она не попадает, но проверка стоит,
		// чтобы это не превратилось молча в узел без тела.
		if hop.Scheme == configtypes.SchemeGroup {
			continue
		}
		tag := relayTagFor(ownerTag, i, len(e.Node.Chain))
		body, err := emitMigrationBody(hop)
		if err != nil {
			// Релей не собрался — владелец останется БЕЗ detour и пойдёт
			// напрямую. Это честнее, чем ссылка в никуда: прямой путь может
			// не работать, но fail-closed на служебном узле оставил бы
			// человека вообще без узла и без объяснения.
			continue
		}
		out = append(out, state.Node{
			Kind:    state.SourceKindServer,
			Tag:     tag,
			Enabled: true,
			Service: true,
			Body:    body,
			// Каждый следующий релей — дозвон предыдущего; последний идёт
			// напрямую, поэтому Detour у него пуст.
			Detour: relayNextLink(subID, ownerTag, i, len(e.Node.Chain)),
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, &state.NodeLink{FolderID: subID, Tag: out[0].Tag}
}

// relayTagFor — имя служебного узла: от владельца, с номером при нескольких.
func relayTagFor(ownerTag string, idx, total int) string {
	if total <= 1 {
		return ownerTag + relayTagSuffix
	}
	return fmt.Sprintf("%s%s %d", ownerTag, relayTagSuffix, idx+1)
}

// relayNextLink — ссылка на следующий релей цепочки; nil у последнего.
func relayNextLink(subID, ownerTag string, idx, total int) *state.NodeLink {
	if idx+1 >= total {
		return nil
	}
	return &state.NodeLink{FolderID: subID, Tag: relayTagFor(ownerTag, idx+1, total)}
}
