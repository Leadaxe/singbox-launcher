package subscription

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// Pasted WireGuard/AmneziaWG .conf import (SPEC 076).
//
// A multi-line [Interface]/[Peer] text pasted into the Sources Add field is not
// a URI and would be destroyed by per-line classification. These helpers carve
// the conf blocks out of the pasted text and convert each to the canonical
// wireguard:// URI (SPEC 075 converter), so downstream storage/parse/share
// paths stay URI-only. AWG fields and the AWG MTU clamp are handled by
// the registry mapper section as usual.

// ExtractWGConfBlocks splits pasted text into [Interface]/[Peer] blocks and the
// remaining text. A block starts at a line equal to "[Interface]" (case-
// insensitive, as wg-quick treats section names) and runs until the next such
// line. Text before the first block is returned as rest for the normal
// line-by-line classification, so links and conf text can be pasted together.
// With no [Interface] line, rest == input and blocks is empty.
func ExtractWGConfBlocks(input string) (rest string, blocks []string) {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	var restLines, block []string
	inBlock := false
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "[interface]") {
			if inBlock {
				blocks = append(blocks, strings.Join(block, "\n"))
			}
			block = []string{line}
			inBlock = true
			continue
		}
		if inBlock {
			block = append(block, line)
		} else {
			restLines = append(restLines, line)
		}
	}
	if inBlock {
		blocks = append(blocks, strings.Join(block, "\n"))
	}
	return strings.Join(restLines, "\n"), blocks
}

// ConvertWGConfText converts one [Interface]/[Peer] block to the canonical
// wireguard:// URI accepted by ParseNode.
//
// Ссылка строится ИЗ ТЕЛА, собранного секцией реестра (SPEC 133): текст
// `.conf` разбирает движок, а эмиттер схемы печатает результат обратно
// ссылкой. Рукописный конвертер (~90 строк перечисления ключей, где каждый
// новый набор — AWG3, сахар маскировки — приходилось дописывать вторым
// списком поверх объявленного в реестре) снят.
//
// Метку считает та же секция своей цепочкой `label.source`
// (`ini.$comment.Peer` → `hint` → хост Endpoint), а не правило, написанное
// здесь во второй раз: провайдеры пишут локацию голым комментарием сразу
// под [Peer], и узел с таким файлом не должен называться «194.180.34.8».
func ConvertWGConfText(confText string) (string, error) {
	node, err, known := ParseWGConfByEngine(confText, nil)
	if !known {
		return "", fmt.Errorf("not a WireGuard config")
	}
	if err != nil {
		return "", err
	}
	if node == nil {
		return "", fmt.Errorf("wg-quick block parsed to no node")
	}
	return ShareURIFromWireGuardEndpoint(node.Outbound)
}

// WGConfBodyToURIs превращает тело подписки формата wg-quick в список
// канонических wireguard://-URI (SPEC 103 B11, BodyKindWGConf).
//
// Один conf может нести несколько секций [Interface] — провайдеры так отдают
// набор локаций одним файлом. Битый блок пропускается с сообщением, остальные
// разбираются: политика та же, что у испорченной строки URI-списка, — одна
// плохая запись не должна обнулять подписку целиком.
//
// Возвращает URI и число пропущенных блоков, чтобы вызывающий мог сказать
// пользователю, сколько узлов потеряно, а не молчать.
func WGConfBodyToURIs(body string) (uris []string, skipped int) {
	converted, skipped := WGConfBodyToConvertedBlocks(body)
	uris = make([]string, 0, len(converted))
	for _, c := range converted {
		// Битый блок остаётся в списке блоков (у него свой Raw и причина —
		// они нужны разбору тела), но ССЫЛКОЙ он не стал: сюда ему нечего
		// отдать, и число потерянных вызывающий получает через skipped.
		if c.Err != nil || c.URI == "" {
			continue
		}
		uris = append(uris, c.URI)
	}
	return uris, skipped
}

// WGConfBlocksOf возвращает блоки [Interface] текста, если он ими является.
//
// Отвечает на вопрос «это конфиг wg-quick, а не URI?» — тем же признаком,
// которым тело классифицируется при разборе подписки (наличие секции
// [Interface]). Текст без единой секции даёт nil: вызывающий разбирает его
// как URI, ничего не меняя в своём поведении.
func WGConfBlocksOf(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, blocks := ExtractWGConfBlocks(text)
	return blocks
}

// ConvertedWGBlock — блок wg-quick вместе с URI, в который он превратился.
//
// Пара нужна разбору тела: узел собирается из URI, а происхождением его
// обязан стать ИСХОДНЫЙ текст блока (SPEC 119). Канонический
// wireguard://-URI — результат нашей конвертации, а не то, что прислал
// провайдер: в нём нет ни комментариев блока (метка локации, выключенные
// опции, закомментированный запасной Endpoint), ни исходного порядка
// ключей. Узел, у которого в origin лежит такая ссылка, невозможно
// пересобрать из конфига провайдера — Regen работал бы по нашему же
// выводу, а не по источнику.
type ConvertedWGBlock struct {
	// URI — канонический wireguard://, которым узел разбирается дальше.
	// Пусто у блока, который в ссылку не превратился (см. Err).
	URI string
	// Raw — блок [Interface]…[Peer] БАЙТ В БАЙТ, как он стоял в теле.
	Raw string
	// Node — узел, собранный секцией реестра ИЗ БЛОКА, а не из URI.
	//
	// Разбирать обратно свою же ссылку значило бы терять то, чего в ней
	// нет по построению: DNS в тело не едет вовсе (код
	// `wgconf_dns_ignored`), а метка из комментария `[Peer]` живёт во
	// фрагменте, которого у канонической ссылки может не быть.
	Node *configtypes.ParsedNode
	// Err — почему блок не стал ссылкой; nil у собравшихся.
	//
	// Блок с ошибкой ОСТАЁТСЯ в списке, на своём месте: разбор тела обязан
	// материализовать каждую запись, не ставшую узлом, — на её позиции, с
	// исходником и причиной (SPEC 116 W11). Молчаливый пропуск оставлял бы
	// человека с телом из трёх блоков и двумя узлами без объяснения, куда
	// делся третий.
	Err error
}

// WGConfBodyToConvertedBlocks конвертирует тело wg-quick, сохраняя за каждым
// URI его исходный блок.
//
// Битые блоки возвращаются НАРАВНЕ с собравшимися — со своим Raw и причиной
// в Err, на своих местах; skipped считает их для сводного warning'а.
func WGConfBodyToConvertedBlocks(body string) (converted []ConvertedWGBlock, skipped int) {
	_, blocks := ExtractWGConfBlocks(body)
	converted = make([]ConvertedWGBlock, 0, len(blocks))
	for _, block := range blocks {
		// Узел собирает СЕКЦИЯ из самого блока (SPEC 133). Ссылка рядом
		// остаётся для тех, кто умеет только URI-список, но узлом из неё
		// больше не разбираются: обратный перевод теряет то, чего в ссылке
		// нет по построению — код `wgconf_dns_ignored` (DNS в тело не
		// едет) и метку из комментария.
		node, err, known := ParseWGConfByEngine(block, nil)
		if err == nil && !known {
			err = fmt.Errorf("not a WireGuard config")
		}
		if err != nil {
			skipped++
			converted = append(converted, ConvertedWGBlock{Raw: block, Err: err})
			continue
		}
		uri := ""
		if node != nil {
			uri, _ = ShareURIFromWireGuardEndpoint(node.Outbound)
		}
		converted = append(converted, ConvertedWGBlock{URI: uri, Raw: block, Node: node})
	}
	return converted, skipped
}
