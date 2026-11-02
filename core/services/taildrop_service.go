// File taildrop_service.go — служебные файлы поверх Taildrop (SPEC 176):
// команды получателю обычным файлом `lx-<время>-<метка>.json`. Норма —
// contract/docs/SERVICE_FILES.md, константы — contract/registry/service_files.json
// (читаются из реестра, в коде их нет).
//
// Здесь только кодек: кандидат по имени, разбор тела, сборка тела отправителя.
// Что выполнять и с какими разрешениями — core/taildrop_service.go.
package services

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"singbox-launcher/contract"
)

// Виды команд служебного файла (kinds реестра).
const (
	TaildropServiceClipboard = "clipboard"
	TaildropServiceOpenURL   = "open_url"
)

// TaildropServiceCommand — команда служебного файла. Поля вида — плоско:
// у clipboard — Text, у open_url — URL. Неизвестный вид сохраняется с
// пустыми полями: получатель его пропускает.
type TaildropServiceCommand struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

// TaildropServiceRules — константы протокола из реестра.
type TaildropServiceRules struct {
	NamePrefix      string
	NameSuffix      string
	NameLabelLength int
	MaxBytes        int64
	Versions        []int
	TTL             time.Duration
	URLSchemes      []string
}

var (
	taildropServiceOnce  sync.Once
	taildropServiceRules TaildropServiceRules
	taildropServiceErr   error
)

// TaildropServiceRulesFromRegistry — константы протокола; реестр вшит в
// бинарь, ошибка — только порча сборки.
func TaildropServiceRulesFromRegistry() (TaildropServiceRules, error) {
	taildropServiceOnce.Do(func() {
		taildropServiceRules, taildropServiceErr = loadTaildropServiceRules()
	})
	return taildropServiceRules, taildropServiceErr
}

func loadTaildropServiceRules() (TaildropServiceRules, error) {
	raw, err := contract.Registry.ReadFile("registry/service_files.json")
	if err != nil {
		return TaildropServiceRules{}, err
	}
	var doc struct {
		ServiceFiles struct {
			NamePrefix      struct{ Value string } `json:"name_prefix"`
			NameSuffix      struct{ Value string } `json:"name_suffix"`
			NameLabelLength struct{ Value int }    `json:"name_label_length"`
			MaxBytes        struct{ Value int64 }  `json:"max_bytes"`
			Versions        struct{ Value []int }  `json:"versions"`
			TTLSeconds      struct{ Value int }    `json:"ttl_seconds"`
			Kinds           struct {
				OpenURL struct {
					Schemes []string `json:"schemes"`
				} `json:"open_url"`
			} `json:"kinds"`
		} `json:"service_files"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return TaildropServiceRules{}, fmt.Errorf("service_files.json: %w", err)
	}
	sf := doc.ServiceFiles
	r := TaildropServiceRules{
		NamePrefix:      sf.NamePrefix.Value,
		NameSuffix:      sf.NameSuffix.Value,
		NameLabelLength: sf.NameLabelLength.Value,
		MaxBytes:        sf.MaxBytes.Value,
		Versions:        sf.Versions.Value,
		TTL:             time.Duration(sf.TTLSeconds.Value) * time.Second,
		URLSchemes:      sf.Kinds.OpenURL.Schemes,
	}
	if r.NamePrefix == "" || r.NameSuffix == "" || r.MaxBytes <= 0 || len(r.Versions) == 0 || r.TTL <= 0 {
		return TaildropServiceRules{}, errors.New("service_files.json: incomplete constants")
	}
	return r, nil
}

// TaildropServiceCandidate — может ли файл входящих быть служебным: имя с
// префиксом и суффиксом (точный вид не проверяется — ядро может переименовать
// файл при совпадении), размер не больше предела.
func (r TaildropServiceRules) TaildropServiceCandidate(name string, size int64) bool {
	return strings.HasPrefix(name, r.NamePrefix) && strings.HasSuffix(name, r.NameSuffix) &&
		size >= 0 && size <= r.MaxBytes
}

// ParseTaildropService разбирает тело кандидата. ok=false — обычный файл
// (§4 контракта): больше предела, не UTF-8, не JSON, `lx` нет или он не из
// versions, `commands` нет или не список.
func (r TaildropServiceRules) ParseTaildropService(body []byte) (cmds []TaildropServiceCommand, ok bool) {
	if int64(len(body)) > r.MaxBytes || !utf8.Valid(body) {
		return nil, false
	}
	var doc struct {
		LX       json.RawMessage   `json:"lx"`
		Commands []json.RawMessage `json:"commands"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || doc.Commands == nil {
		return nil, false
	}
	// lx — только число JSON: строка "1" версией не считается.
	var v int
	if len(doc.LX) == 0 || doc.LX[0] == '"' || json.Unmarshal(doc.LX, &v) != nil || !containsInt(r.Versions, v) {
		return nil, false
	}
	for _, raw := range doc.Commands {
		var c TaildropServiceCommand
		if json.Unmarshal(raw, &c) != nil {
			// Команда — не объект нужного вида: пропускается, как неизвестный kind.
			c = TaildropServiceCommand{}
		}
		cmds = append(cmds, c)
	}
	return cmds, true
}

// ClipboardText — текст команды clipboard к записи в буфер: один ведущий
// U+FEFF снимается; пустой — команды нет.
func (c TaildropServiceCommand) ClipboardText() (string, bool) {
	text := strings.TrimPrefix(c.Text, "\uFEFF")
	return text, text != ""
}

// OpenURLAllowed — ссылка команды open_url со схемой из реестра.
func (r TaildropServiceRules) OpenURLAllowed(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	for _, s := range r.URLSchemes {
		if strings.EqualFold(u.Scheme, s) {
			return true
		}
	}
	return false
}

// SingleURL — текст целиком одна ссылка со схемой из реестра (кнопка
// «Open on <device>» отправителя).
func (r TaildropServiceRules) SingleURL(text string) (string, bool) {
	t := strings.TrimSpace(text)
	if t == "" || strings.ContainsAny(t, " \t\r\n") || !r.OpenURLAllowed(t) {
		return "", false
	}
	return t, true
}

// Отказы отправителя (§7 п.2 контракта) — человеку называется причина.
var (
	ErrTaildropServiceEmpty   = errors.New("nothing to send: the text is empty")
	ErrTaildropServiceTooBig  = errors.New("the text is too large to send")
	ErrTaildropServiceNotUTF8 = errors.New("the text is not valid UTF-8")
)

// BuildTaildropService — имя и тело служебного файла с одной командой.
// now — часы отправителя (время в имени — UTC).
func (r TaildropServiceRules) BuildTaildropService(cmd TaildropServiceCommand, now time.Time) (string, []byte, error) {
	switch cmd.Kind {
	case TaildropServiceClipboard:
		if cmd.Text == "" {
			return "", nil, ErrTaildropServiceEmpty
		}
		if !utf8.ValidString(cmd.Text) {
			return "", nil, ErrTaildropServiceNotUTF8
		}
	case TaildropServiceOpenURL:
		if !r.OpenURLAllowed(cmd.URL) {
			return "", nil, fmt.Errorf("not an http(s) link")
		}
	default:
		return "", nil, fmt.Errorf("unknown command %q", cmd.Kind)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Текст уходит как есть: <, >, & не экранируются.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(struct {
		LX       int                      `json:"lx"`
		Commands []TaildropServiceCommand `json:"commands"`
	}{LX: r.Versions[len(r.Versions)-1], Commands: []TaildropServiceCommand{cmd}}); err != nil {
		return "", nil, err
	}
	body := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if int64(len(body)) > r.MaxBytes {
		return "", nil, ErrTaildropServiceTooBig
	}
	label, err := randomLabel(r.NameLabelLength)
	if err != nil {
		return "", nil, err
	}
	name := r.NamePrefix + now.UTC().Format("20060102T150405Z") + "-" + label + r.NameSuffix
	return name, body, nil
}

const labelAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomLabel(n int) (string, error) {
	if n <= 0 {
		n = 5
	}
	b := make([]byte, n)
	for i := range b {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(labelAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = labelAlphabet[k.Int64()]
	}
	return string(b), nil
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
