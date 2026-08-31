package subscription

import (
	"fmt"
	"strings"
)

// Pasted WireGuard/AmneziaWG .conf import (SPEC 076).
//
// A multi-line [Interface]/[Peer] text pasted into the Sources Add field is not
// a URI and would be destroyed by per-line classification. These helpers carve
// the conf blocks out of the pasted text and convert each to the canonical
// wireguard:// URI (SPEC 075 converter), so downstream storage/parse/share
// paths stay URI-only. AWG fields and the AWG MTU clamp are handled by
// parseWireGuardURI as usual.

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
// wireguard:// URI accepted by ParseNode. The URI fragment (node label) is the
// Endpoint host: a pasted .conf carries no display name, and a fixed fallback
// would give every pasted node the same tag.
func ConvertWGConfText(confText string) (string, error) {
	_, peer := parseWGConfSections(confText)
	label := wgEndpointHost(peer["endpoint"])
	if label == "" {
		return "", fmt.Errorf("missing required fields: [Peer] endpoint")
	}
	return wgConfToURI(confText, label)
}

// wgEndpointHost extracts the host from a host:port endpoint ("" if malformed).
// IPv6 brackets are stripped: "[2001:db8::1]:51820" → "2001:db8::1".
func wgEndpointHost(endpoint string) string {
	i := strings.LastIndex(endpoint, ":")
	if i <= 0 {
		return ""
	}
	host := strings.TrimSpace(endpoint[:i])
	return strings.Trim(host, "[]")
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
	URI string
	// Raw — блок [Interface]…[Peer] БАЙТ В БАЙТ, как он стоял в теле.
	Raw string
}

// WGConfBodyToConvertedBlocks конвертирует тело wg-quick, сохраняя за каждым
// URI его исходный блок. Битые блоки пропускаются и считаются в skipped —
// политика та же, что у WGConfBodyToURIs.
func WGConfBodyToConvertedBlocks(body string) (converted []ConvertedWGBlock, skipped int) {
	_, blocks := ExtractWGConfBlocks(body)
	converted = make([]ConvertedWGBlock, 0, len(blocks))
	for _, block := range blocks {
		uri, err := ConvertWGConfText(block)
		if err != nil {
			skipped++
			continue
		}
		converted = append(converted, ConvertedWGBlock{URI: uri, Raw: block})
	}
	return converted, skipped
}
