// File config_gate.go — гейт ключей config.json ВНЕ тел узлов по версии ядра
// (registry/core_config.json, контракт 1.1.163).
//
// Поля узлов гейтит GateForCore по `min_core` схемы протокола. Но шаблон
// пишет в конфиг и то, что узлом не является, — корневой блок `lx` с
// настройками форка. Ключ, которого ядро не знает, там такой же отказ ВСЕГО
// конфига («unknown field»), а исполнить конфиг может ядро старше пина
// лаунчера: служба со своей копией ядра, удалённая машина.
package nodeflow

import (
	"encoding/json"
	"sort"
	"strings"

	"singbox-launcher/core/config/registry"
)

// GateConfigForCore снимает из корня конфига ключи, которых ядро версии
// version не знает, и возвращает снятые пути. root правится на месте;
// объект, опустевший от снятия, снимается тоже.
//
// Пустая version — гейт не применяется: не деградируем по догадке, та же
// политика, что у полевого гейта узлов.
func GateConfigForCore(root map[string]json.RawMessage, version string) ([]string, error) {
	version = strings.TrimSpace(version)
	if version == "" || len(root) == 0 {
		return nil, nil
	}
	reg, err := registry.Get()
	if err != nil {
		return nil, err
	}
	fields := reg.CoreConfigFields()
	paths := make([]string, 0, len(fields))
	for path, f := range fields {
		if f.MinCore != "" && compareCoreVersions(version, f.MinCore) < 0 {
			paths = append(paths, path)
		}
	}
	// Родитель раньше потомка: снятый блок целиком уносит и свои ключи.
	sort.Strings(paths)
	var dropped []string
	for _, path := range paths {
		if deleteRawPath(root, strings.Split(path, ".")) {
			dropped = append(dropped, path)
		}
	}
	return dropped, nil
}

// deleteRawPath удаляет ключ по пути через вложенные объекты; true — ключ
// был и снят. Не-объект на пути — пути нет.
func deleteRawPath(m map[string]json.RawMessage, parts []string) bool {
	raw, ok := m[parts[0]]
	if !ok {
		return false
	}
	if len(parts) == 1 {
		delete(m, parts[0])
		return true
	}
	var child map[string]json.RawMessage
	if err := json.Unmarshal(raw, &child); err != nil || child == nil {
		return false
	}
	if !deleteRawPath(child, parts[1:]) {
		return false
	}
	if len(child) == 0 {
		delete(m, parts[0])
		return true
	}
	out, err := json.Marshal(child)
	if err != nil {
		return false
	}
	m[parts[0]] = out
	return true
}
