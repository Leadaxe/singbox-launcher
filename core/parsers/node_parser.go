// Package parsers provides parsing logic for various proxy node formats.
// It supports VLESS, VMess, Trojan, and Shadowsocks protocols, handling
// both direct links and subscription formats.
package parsers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ParsedNode represents a parsed proxy node with all extracted information.
// It contains protocol-specific fields (UUID, Flow, etc.) and the generated
// outbound configuration ready for JSON serialization.
type ParsedNode struct {
	Tag      string
	Scheme   string
	Server   string
	Port     int
	UUID     string
	Flow     string
	Label    string
	Comment  string
	Query    url.Values
	Outbound map[string]interface{}
}

// IsDirectLink checks if the input string is a direct proxy link (vless://, vmess://, etc.)
func IsDirectLink(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "vless://") ||
		strings.HasPrefix(trimmed, "vmess://") ||
		strings.HasPrefix(trimmed, "trojan://") ||
		strings.HasPrefix(trimmed, "ss://")
}

// ParseNode parses a single node URI and applies skip filters
func ParseNode(uri string, skipFilters []map[string]string) (*ParsedNode, error) {
	// Determine scheme
	scheme := ""
	uriToParse := uri
	var ssMethod, ssPassword string // For SS links: method and password extracted from base64

	// Handle VMess base64 format
	if strings.HasPrefix(uri, "vmess://") {
		scheme = "vmess"
		// VMess is in base64 format, decode with padding support
		base64Part := strings.TrimPrefix(uri, "vmess://")

		// Decode base64 with padding support
		decoded, err := decodeBase64WithPadding(base64Part)
		if err != nil {
			uriPreview := uri
			if len(uriPreview) > 50 {
				uriPreview = uriPreview[:50] + "..."
			}
			log.Printf("Parser: Error: Failed to decode VMESS base64 (uri length: %d, base64 length: %d): %v. URI: %s. Skipping node.",
				len(uri), len(base64Part), err, uriPreview)
			return nil, fmt.Errorf("failed to decode VMESS base64: %w", err)
		}

		if len(decoded) == 0 {
			log.Printf("Parser: Error: VMESS decoded content is empty. Skipping node.")
			return nil, fmt.Errorf("VMESS decoded content is empty")
		}

		// Parse as JSON VMess config
		var vmessConfig map[string]interface{}
		if err := json.Unmarshal(decoded, &vmessConfig); err != nil {
			log.Printf("Parser: Error: Failed to parse VMESS JSON (decoded length: %d): %v. Skipping node.",
				len(decoded), err)
			return nil, fmt.Errorf("failed to parse VMESS JSON: %w", err)
		}

		// Parse VMess JSON configuration
		return parseVMessJSON(vmessConfig, skipFilters)
	} else if strings.HasPrefix(uri, "vless://") {
		scheme = "vless"
	} else if strings.HasPrefix(uri, "trojan://") {
		scheme = "trojan"
	} else if strings.HasPrefix(uri, "ss://") {
		scheme = "ss"

		// SS links in SIP002 format: ss://base64(method:password)@server:port#tag
		ssPart := strings.TrimPrefix(uri, "ss://")

		// Check if it's SIP002 format (has @)
		if atIdx := strings.Index(ssPart, "@"); atIdx > 0 {
			// SIP002: ss://base64(method:password)@server:port#tag
			encodedUserinfo := ssPart[:atIdx]
			rest := ssPart[atIdx+1:]

			// Decode base64 userinfo (with padding support)
			decoded, err := decodeBase64WithPadding(encodedUserinfo)
			if err != nil {
				log.Printf("Parser: Error: Failed to decode SS base64 userinfo. Encoded: %s, Error: %v", encodedUserinfo, err)
			} else {
				// Split method:password
				decodedStr := string(decoded)
				userinfoParts := strings.SplitN(decodedStr, ":", 2)
				if len(userinfoParts) == 2 {
					ssMethod = userinfoParts[0]
					ssPassword = userinfoParts[1]
					log.Printf("Parser: Successfully extracted SS credentials: method=%s, password length=%d", ssMethod, len(ssPassword))
				} else {
					log.Printf("Parser: Error: SS decoded userinfo doesn't contain ':' separator. Decoded: %s", decodedStr)
				}
			}

			// Reconstruct URI for standard parsing
			uriToParse = "ss://" + rest
		} else {
			log.Printf("Parser: Warning: SS link is not in SIP002 format (no @ found): %s", uri)
		}
	} else {
		return nil, fmt.Errorf("unsupported scheme")
	}

	// Parse URI
	parsedURL, err := url.Parse(uriToParse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	// Extract components
	node := &ParsedNode{
		Scheme: scheme,
		Server: parsedURL.Hostname(),
		Query:  parsedURL.Query(),
	}

	// For SS, store method and password in Query (if extracted during parsing)
	if scheme == "ss" {
		if ssMethod == "" || ssPassword == "" {
			log.Printf("Parser: Error: SS link missing method or password. URI: %s", uri)
			return nil, fmt.Errorf("SS link missing required method or password")
		}
		node.Query.Set("method", ssMethod)
		node.Query.Set("password", ssPassword)
	}

	// Extract port
	port := parsedURL.Port()
	if port == "" {
		// Default ports
		switch scheme {
		case "vless", "vmess":
			node.Port = 443
		case "trojan":
			node.Port = 443
		case "ss":
			node.Port = 443
		}
	} else {
		if p, err := strconv.Atoi(port); err == nil {
			node.Port = p
		} else {
			node.Port = 443 // Fallback
		}
	}

	// Extract UUID/user
	if parsedURL.User != nil {
		node.UUID = parsedURL.User.Username()
	}

	// Extract fragment (label)
	node.Label = parsedURL.Fragment
	// URL decode the fragment if needed
	if node.Label != "" {
		if decoded, err := url.QueryUnescape(node.Label); err == nil {
			node.Label = decoded
		}
	}

	// For some formats, label might be in path or userinfo
	if node.Label == "" {
		// Try to extract from path (some formats use path for label)
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			node.Label = strings.TrimPrefix(parsedURL.Path, "/")
		} else if parsedURL.User != nil {
			// Some formats encode label in username
			node.Label = parsedURL.User.Username()
		}
	}

	// Extract tag and comment from label
	node.Tag, node.Comment = extractTagAndComment(node.Label)

	// Normalize flag
	node.Tag = normalizeFlagTag(node.Tag)

	// Extract flow
	node.Flow = parsedURL.Query().Get("flow")

	// Apply skip filters
	if shouldSkipNode(node, skipFilters) {
		return nil, nil // Node should be skipped
	}

	// Build outbound JSON based on scheme
	node.Outbound = buildOutbound(node)

	return node, nil
}

// Private helper functions (migrated from parser.go)

func decodeBase64WithPadding(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(s)
	}
	return decoded, err
}

func extractTagAndComment(label string) (tag, comment string) {
	// Tag should contain the full label (including part after |)
	tag = label

	// Comment contains only the part after | (if exists)
	parts := strings.Split(label, "|")
	if len(parts) > 1 {
		comment = strings.Join(parts[1:], "|") // Join in case there are multiple |
	} else {
		comment = label // If no |, use full label as comment
	}
	return strings.TrimSpace(tag), strings.TrimSpace(comment)
}

func normalizeFlagTag(tag string) string {
	return strings.ReplaceAll(tag, "🇪🇳", "🇬🇧")
}

func shouldSkipNode(node *ParsedNode, skipFilters []map[string]string) bool {
	for _, filter := range skipFilters {
		allKeysMatch := true
		for key, pattern := range filter {
			value := getNodeValue(node, key)
			if !matchesPattern(value, pattern) {
				allKeysMatch = false
				break
			}
		}
		if allKeysMatch {
			return true // Skip node
		}
	}
	return false // Don't skip
}

func getNodeValue(node *ParsedNode, key string) string {
	switch key {
	case "tag":
		return node.Tag
	case "host":
		return node.Server
	case "label":
		return node.Label
	case "scheme":
		return node.Scheme
	case "fragment":
		return node.Label // fragment == label
	case "comment":
		return node.Comment
	default:
		return ""
	}
}

func matchesPattern(value, pattern string) bool {
	// Negation literal: !literal
	if strings.HasPrefix(pattern, "!") && !strings.HasPrefix(pattern, "!/") {
		literal := strings.TrimPrefix(pattern, "!")
		return value != literal
	}

	// Negation regex: !/regex/i
	if strings.HasPrefix(pattern, "!/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "!/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			log.Printf("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return !re.MatchString(value)
	}

	// Regex: /regex/i
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			log.Printf("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return re.MatchString(value)
	}

	// Literal match
	return value == pattern
}

func buildOutbound(node *ParsedNode) map[string]interface{} {
	outbound := make(map[string]interface{})
	outbound["tag"] = node.Tag
	// Use "shadowsocks" instead of "ss" for sing-box
	if node.Scheme == "ss" {
		outbound["type"] = "shadowsocks"
	} else {
		outbound["type"] = node.Scheme
	}
	outbound["server"] = node.Server
	outbound["server_port"] = node.Port

	if node.Scheme == "vless" {
		outbound["uuid"] = node.UUID
		if node.Flow != "" {
			outbound["flow"] = node.Flow
		}

		// Build TLS structure with correct field order
		sni := node.Query.Get("sni")
		if sni == "" {
			sni = node.Server // Fallback to server hostname
		}
		fp := node.Query.Get("fp")
		if fp == "" {
			fp = "random"
		}
		pbk := node.Query.Get("pbk")
		sid := node.Query.Get("sid")

		tlsData := map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
		}

		if pbk != "" {
			tlsData["reality"] = map[string]interface{}{
				"enabled":    true,
				"public_key": pbk,
				"short_id":   sid,
			}
		}

		outbound["tls"] = tlsData
	} else if node.Scheme == "vmess" {
		outbound["uuid"] = node.UUID

		if security := node.Query.Get("security"); security != "" {
			outbound["security"] = security
		} else {
			outbound["security"] = "auto" // default
		}

		if alterIDStr := node.Query.Get("alter_id"); alterIDStr != "" {
			if alterID, err := strconv.Atoi(alterIDStr); err == nil {
				outbound["alter_id"] = alterID
			}
		}

		network := node.Query.Get("network")
		if network == "" {
			network = "tcp" // default
		}

		if network == "ws" || network == "http" || network == "grpc" {
			transport := make(map[string]interface{})
			transport["type"] = network

			if path := node.Query.Get("path"); path != "" {
				transport["path"] = path
			}

			if network == "ws" || network == "http" {
				if host := node.Query.Get("host"); host != "" {
					headers := map[string]string{"Host": host}
					transport["headers"] = headers
				}
			}

			outbound["transport"] = transport
		}

		if node.Query.Get("tls_enabled") == "true" {
			tlsData := map[string]interface{}{
				"enabled": true,
			}

			if sni := node.Query.Get("sni"); sni != "" {
				tlsData["server_name"] = sni
			}

			if alpn := node.Query.Get("alpn"); alpn != "" {
				alpnList := strings.Split(alpn, ",")
				for i, a := range alpnList {
					alpnList[i] = strings.TrimSpace(a)
				}
				tlsData["alpn"] = alpnList
			}

			if fp := node.Query.Get("fp"); fp != "" {
				tlsData["utls"] = map[string]interface{}{
					"enabled":     true,
					"fingerprint": fp,
				}
			}

			if node.Query.Get("insecure") == "true" {
				tlsData["insecure"] = true
			}

			outbound["tls"] = tlsData
		}
	} else if node.Scheme == "trojan" {
		outbound["password"] = node.UUID
	} else if node.Scheme == "ss" {
		if method := node.Query.Get("method"); method != "" {
			outbound["method"] = method
		}
		if password := node.Query.Get("password"); password != "" {
			outbound["password"] = password
		}
	}

	return outbound
}

func parseVMessJSON(vmessConfig map[string]interface{}, skipFilters []map[string]string) (*ParsedNode, error) {
	node := &ParsedNode{
		Scheme: "vmess",
		Query:  make(url.Values),
	}

	var missingFields []string

	if add, ok := vmessConfig["add"].(string); ok && add != "" {
		node.Server = add
	} else {
		missingFields = append(missingFields, "add")
	}

	if port, ok := vmessConfig["port"].(float64); ok {
		node.Port = int(port)
	} else if portStr, ok := vmessConfig["port"].(string); ok {
		if p, err := strconv.Atoi(portStr); err == nil {
			node.Port = p
		} else {
			missingFields = append(missingFields, "port (invalid format)")
		}
	} else {
		missingFields = append(missingFields, "port")
	}

	if id, ok := vmessConfig["id"].(string); ok && id != "" {
		node.UUID = id
	} else {
		missingFields = append(missingFields, "id")
	}

	if len(missingFields) > 0 {
		return nil, fmt.Errorf("missing required fields: %v", missingFields)
	}

	if ps, ok := vmessConfig["ps"].(string); ok && ps != "" {
		node.Label = ps
		node.Tag, node.Comment = extractTagAndComment(ps)
		node.Tag = normalizeFlagTag(node.Tag)
	} else {
		node.Tag = fmt.Sprintf("vmess-%s-%d", node.Server, node.Port)
		node.Comment = node.Tag
	}

	if scy, ok := vmessConfig["scy"].(string); ok && scy != "" {
		node.Query.Set("security", scy)
	} else {
		node.Query.Set("security", "auto")
	}

	if aid, ok := vmessConfig["aid"].(string); ok && aid != "" && aid != "0" {
		node.Query.Set("alter_id", aid)
	} else if aidNum, ok := vmessConfig["aid"].(float64); ok && aidNum != 0 {
		node.Query.Set("alter_id", strconv.Itoa(int(aidNum)))
	}

	net := ""
	if netVal, ok := vmessConfig["net"].(string); ok && netVal != "" {
		net = netVal
		if net == "xhttp" {
			net = "ws"
		}
		node.Query.Set("network", net)
	} else {
		net = "tcp"
		node.Query.Set("network", net)
	}

	if path, ok := vmessConfig["path"].(string); ok && path != "" {
		node.Query.Set("path", path)
	}

	if host, ok := vmessConfig["host"].(string); ok && host != "" {
		node.Query.Set("host", host)
	}

	if tls, ok := vmessConfig["tls"].(string); ok && tls == "tls" {
		node.Query.Set("tls_enabled", "true")

		sni := ""
		if sniVal, ok := vmessConfig["sni"].(string); ok && sniVal != "" {
			sni = sniVal
		} else if host, ok := vmessConfig["host"].(string); ok && host != "" {
			sni = host
		} else {
			sni = node.Server
		}
		node.Query.Set("sni", sni)

		if alpn, ok := vmessConfig["alpn"].(string); ok && alpn != "" {
			node.Query.Set("alpn", alpn)
		}

		if fp, ok := vmessConfig["fp"].(string); ok && fp != "" {
			node.Query.Set("fp", fp)
		}

		if insecure, ok := vmessConfig["insecure"].(string); ok && insecure == "1" {
			node.Query.Set("insecure", "true")
		}
	}

	if shouldSkipNode(node, skipFilters) {
		return nil, nil // Skip node
	}

	node.Outbound = buildOutbound(node)
	return node, nil
}
