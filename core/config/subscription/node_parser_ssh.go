package subscription

import (
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// buildSSHOutbound builds outbound configuration for SSH protocol.
//
// node.Query values are already percent-decoded by url.Parse — re-decoding
// them (historically via url.QueryUnescape) corrupted values containing '+'
// (turned into a space) or literal %XX sequences. PEM private keys are the
// worst hit: their base64 body is full of '+'.
func buildSSHOutbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	// User is required (stored in UUID field from userinfo)
	if node.UUID != "" {
		outbound["user"] = node.UUID
	} else {
		outbound["user"] = "root" // Default user for SSH
		debuglog.WarnLog("Parser: SSH link missing user, using default 'root'")
	}

	// Password is optional (can be in query params from userinfo)
	if password := node.Query.Get("password"); password != "" {
		outbound["password"] = password
	}

	// Private key (inline) - if provided, takes precedence over private_key_path
	if privateKey := node.Query.Get("private_key"); privateKey != "" {
		outbound["private_key"] = privateKey
	} else if privateKeyPath := node.Query.Get("private_key_path"); privateKeyPath != "" {
		outbound["private_key_path"] = privateKeyPath
	}

	// Private key passphrase
	if passphrase := node.Query.Get("private_key_passphrase"); passphrase != "" {
		outbound["private_key_passphrase"] = passphrase
	}

	// Host key (can be multiple, comma-separated)
	if hostKey := node.Query.Get("host_key"); hostKey != "" {
		hostKeys := strings.Split(hostKey, ",")
		keys := make([]string, 0, len(hostKeys))
		for _, key := range hostKeys {
			if key = strings.TrimSpace(key); key != "" {
				keys = append(keys, key)
			}
		}
		if len(keys) > 0 {
			outbound["host_key"] = keys
		}
	}

	// Host key algorithms (can be multiple, comma-separated)
	if algorithms := node.Query.Get("host_key_algorithms"); algorithms != "" {
		algList := strings.Split(algorithms, ",")
		filteredAlgs := make([]string, 0, len(algList))
		for _, alg := range algList {
			if alg = strings.TrimSpace(alg); alg != "" {
				filteredAlgs = append(filteredAlgs, alg)
			}
		}
		if len(filteredAlgs) > 0 {
			outbound["host_key_algorithms"] = filteredAlgs
		}
	}

	// Client version
	if clientVersion := node.Query.Get("client_version"); clientVersion != "" {
		outbound["client_version"] = clientVersion
	}
}
