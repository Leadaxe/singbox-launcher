# План: 044 — NAIVE_PROXY_PARSER

## 1. Parser layer (`core/config/subscription/node_parser.go`)

1.1. `IsDirectLink` +2 строки:
```go
strings.HasPrefix(trimmed, "naive+https://") ||
strings.HasPrefix(trimmed, "naive+quic://")
```

1.2. `ParseNode` dispatch — добавить case'ы:
```go
case strings.HasPrefix(uri, "naive+https://"), strings.HasPrefix(uri, "naive+quic://"):
    scheme = "naive"
    defaultPort = 443
    // Заменяем URI-схему "naive+xxx" на "https"/"quic" для стандартного url.Parse,
    // и запоминаем transport-режим в node.Query["quic"] = "true" (для наглядности).
    isQUIC := strings.HasPrefix(uri, "naive+quic://")
    uriToParse = strings.Replace(uri, "naive+quic://", "https://", 1)
    uriToParse = strings.Replace(uriToParse, "naive+https://", "https://", 1)
```

1.3. После общего `url.Parse` — заполняем specific-for-naive query:

- `node.UUID` ← parsedURL.User.Username() (может быть пустым).
- `node.Query["password"]` ← password из User.Password() (не UUID, иначе перепутается с vless UUID семантикой).
- Если `isQUIC` → `node.Query["quic"] = "true"`.
- `padding` из Query → WARN + **remove** из node.Query (чтобы не утёк в outbound).
- `extra-headers` из Query → оставить как есть; дальше generator парсит.

1.4. `buildOutbound` — новая ветка после `scheme == "ssh"`:

```go
} else if node.Scheme == "naive" {
    buildNaiveOutbound(node, outbound)
}
```

Функция `buildNaiveOutbound` — в новом `node_parser_naive.go`.

## 2. Naive-specific helpers (`core/config/subscription/node_parser_naive.go`)

```go
package subscription

import "strings"

// naiveHeaderNameCharset — allowed chars in HTTP header names per NaïveProxy URI spec (DuckSoft).
const naiveHeaderNameCharset = "!#$%&'*+-.0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ\\^_`abcdefghijklmnopqrstuvwxyz|~"

// parseNaiveExtraHeaders parses URL-decoded "extra-headers" param value into a map.
// Format: "Header1: Value1\r\nHeader2: Value2".
// Invalid pairs are skipped with a warning and do NOT fail the whole parse.
func parseNaiveExtraHeaders(s string) map[string]string {
    if s == "" { return nil }
    out := map[string]string{}
    lines := strings.Split(s, "\r\n")
    for _, line := range lines {
        // split on first ":"
        idx := strings.Index(line, ":")
        if idx <= 0 { /* warn + skip */ continue }
        name := strings.TrimSpace(line[:idx])
        val := strings.TrimSpace(line[idx+1:])
        if !isValidNaiveHeaderName(name) { /* warn + skip */ continue }
        if strings.ContainsAny(val, "\r\n\x00") { /* warn + skip */ continue }
        out[name] = val
    }
    if len(out) == 0 { return nil }
    return out
}

// isValidNaiveHeaderName: each rune must be in naiveHeaderNameCharset.
func isValidNaiveHeaderName(s string) bool { ... }

// buildNaiveOutbound fills outbound map[string]interface{} for Scheme == "naive".
func buildNaiveOutbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
    if node.UUID != "" { outbound["username"] = node.UUID }
    if pw := node.Query.Get("password"); pw != "" { outbound["password"] = pw }
    if node.Query.Get("quic") == "true" {
        outbound["quic"] = true
        outbound["quic_congestion_control"] = "bbr"
    }
    if raw := node.Query.Get("extra-headers"); raw != "" {
        if hdrs := parseNaiveExtraHeaders(raw); len(hdrs) > 0 {
            outbound["extra_headers"] = hdrs
        }
    }
    // TLS (always enabled for naive — no sense without it).
    outbound["tls"] = map[string]interface{}{
        "enabled":     true,
        "server_name": node.Server,
    }
}
```

## 3. Share URI encoder (`core/config/subscription/share_uri_encode.go`)

3.1. `ShareURIFromOutbound` switch:
```go
case "naive":
    return shareURIFromNaive(out)
```

3.2. Новая функция `shareURIFromNaive(out)`:

```go
func shareURIFromNaive(out map[string]interface{}) (string, error) {
    server := mapGetString(out, "server")
    port := mapGetInt(out, "server_port")
    if server == "" || port <= 0 {
        return "", fmt.Errorf("%w: naive needs server, server_port", ErrShareURINotSupported)
    }

    scheme := "naive+https"
    if mapGetBool(out, "quic") {
        scheme = "naive+quic"
    }

    q := url.Values{}
    if hdrs, ok := out["extra_headers"].(map[string]interface{}); ok && len(hdrs) > 0 {
        // sort keys for deterministic output
        keys := make([]string, 0, len(hdrs))
        for k := range hdrs { keys = append(keys, k) }
        sort.Strings(keys)
        var parts []string
        for _, k := range keys {
            v := fmt.Sprint(hdrs[k])
            parts = append(parts, k+": "+v)
        }
        q.Set("extra-headers", strings.Join(parts, "\r\n"))
    }
    shareAppendDetourLiteral(q, out)

    user := mapGetString(out, "username")
    pass := mapGetString(out, "password")
    var ui *url.Userinfo
    switch {
    case user != "" && pass != "":
        ui = url.UserPassword(user, pass)
    case pass != "" && user == "":
        ui = url.User(pass) // spec allows "password only" via user slot
    case user != "" && pass == "":
        ui = url.User(user)
    }

    u := &url.URL{
        Scheme:   scheme,
        User:     ui,
        Host:     hostPort(server, port),
        RawQuery: q.Encode(),
        Fragment: fragmentFromTag(out),
    }
    return u.String(), nil
}
```

**Важно:** `url.URL.String()` сам экранирует query-value, так что `\r\n` в `extra-headers` value стала `%0D%0A`. Проверить в тесте что результат соответствует спеке.

## 4. Тесты

- `core/config/subscription/node_parser_naive_test.go` — новый файл: parser cases (§7.1 SPEC) + buildOutbound roundtrip.
- `core/config/subscription/share_uri_encode_test.go` — дополнить: naive cases (§7.3).

## 5. Документация

- `docs/ParserConfig.md` — раздел «NaïveProxy URI». Формат + 2-3 примера.
- `docs/release_notes/upcoming.md` — запись EN + RU в секцию «Highlights».
- `README.md` / `README_RU.md` — в списке supported protocols.

## 6. Этапы коммитов

Последовательность:

1. `feat(subscription): parse naive+https / naive+quic URIs` — parser + IsDirectLink + helpers + unit tests.
2. `feat(subscription): emit naive outbound JSON + buildNaiveOutbound` — generator + roundtrip tests. (Можно слить с шагом 1, если удобнее одним коммитом.)
3. `feat(share-uri): encode naive outbound back to URI` — encoder + tests.
4. `docs: naive proxy — ParserConfig + release-notes + README` — doc-only коммит.

Альтернативно — **один коммит** "feat(subscription): NaïveProxy support — parser, generator, share URI, docs" — если диффы маленькие.
