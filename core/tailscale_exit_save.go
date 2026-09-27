// File tailscale_exit_save.go — кнопка Save choice вкладки Network (SPEC 148
// §5, LxBox §581 раздел 5): действующий exit node пишется в `exit_node`
// тела узла, конфиг пересобирается. Меняется только это поле.
//
// Писать можно только в СВОЙ узел: свободный сервер в корне или член папки.
// У узла подписки тело принадлежит провайдеру, и правка потерялась бы при
// следующем обновлении — кнопка там скрыта (CanSaveTailscaleExitNode).
package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// tailscaleExitNodeField — поле тела узла tailscale с выходом.
const tailscaleExitNodeField = "exit_node"

// errNoOwnTailscaleNode — узла с таким финальным тегом среди своих нет.
var errNoOwnTailscaleNode = errors.New("the node is not a server of your own: the choice cannot be saved")

// SetBodyTopLevelString ставит строковое поле верхнего уровня JSON-объекта,
// сохраняя порядок остальных ключей; пустое value убирает поле. Новое поле
// дописывается в конец. Результат компактный.
func SetBodyTopLevelString(body []byte, key, value string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("node body is not a JSON object")
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	put := func(k string, v []byte) error {
		kb, err := json.Marshal(k)
		if err != nil {
			return err
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(v)
		return nil
	}
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	replaced := false
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k, _ := kt.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if k == key {
			replaced = true
			if value == "" {
				continue
			}
			raw = valueJSON
		}
		if err := put(k, raw); err != nil {
			return nil, err
		}
	}
	if !replaced && value != "" {
		if err := put(key, valueJSON); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	var out bytes.Buffer
	if err := json.Compact(&out, buf.Bytes()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// findOwnTailscaleNode — свой узел tailscale с финальным тегом finalTag.
func findOwnTailscaleNode(s *state.State, finalTag string) *state.Node {
	isTailscale := func(n *state.Node) bool {
		if n.Kind != state.SourceKindServer || len(n.Body) == 0 {
			return false
		}
		var head struct {
			Type string `json:"type"`
		}
		return json.Unmarshal(n.Body, &head) == nil && head.Type == "tailscale"
	}
	for i := range s.Sources {
		src := &s.Sources[i]
		switch src.Kind {
		case state.SourceKindServer:
			if strings.TrimSpace(src.Tag) == finalTag && isTailscale(&src.Node) {
				return &src.Node
			}
		case state.SourceKindFolder:
			for j := range src.Nodes {
				n := &src.Nodes[j]
				final, ok := state.NodeLinkFinalTag(src.TagPolicy, n.Tag)
				if ok && final == finalTag && isTailscale(n) {
					return n
				}
			}
		}
	}
	return nil
}

// CanSaveTailscaleExitNode — показывать ли Save choice: узел свой и
// локальный (удалённая машина строит конфиг из своего состояния).
func (ac *AppController) CanSaveTailscaleExitNode(finalTag string) bool {
	if ac == nil || ac.FileService == nil {
		return false
	}
	s, err := state.Load(platform.GetWizardStatePath(ac.FileService.Layout.Data))
	if err != nil {
		return false
	}
	return findOwnTailscaleNode(s, finalTag) != nil
}

// SaveTailscaleExitNode пишет value в `exit_node` тела своего узла ("" —
// убирает поле), сохраняет состояние и пересобирает конфиг.
//
// У узла с источником-телом (`singbox_outbound`, вид json) правится текст
// источника, тело материализуется из него — тем же путём, что Apply вкладки
// JSON. У узла из ссылки или INI правится тело, происхождение не меняется
// (MaterializeEditedBody), как у правки тела узла с таким источником.
func (ac *AppController) SaveTailscaleExitNode(finalTag, value string) error {
	if ac == nil || ac.FileService == nil {
		return fmt.Errorf("FileService not initialized")
	}
	statePath := platform.GetWizardStatePath(ac.FileService.Layout.Data)
	s, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	node := findOwnTailscaleNode(s, finalTag)
	if node == nil {
		return errNoOwnTailscaleNode
	}
	jsonOrigin := node.Origin == nil || node.Origin.Kind == state.OriginKindJSON
	src := []byte(node.Body)
	if jsonOrigin && node.Origin != nil && strings.TrimSpace(node.Origin.Raw) != "" {
		src = []byte(node.Origin.Raw)
	}
	edited, err := SetBodyTopLevelString(src, tailscaleExitNodeField, strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("edit node body: %w", err)
	}
	var mat *config.ServerNodeMaterial
	if jsonOrigin {
		mat, err = config.MaterializeServerNode("", json.RawMessage(edited))
	} else {
		mat, err = config.MaterializeEditedBody(json.RawMessage(edited))
	}
	if err != nil {
		return err
	}
	before := node.Body
	node.Body = mat.Body
	node.ReplaceDerivedWarnings(mat.Warnings)
	node.RevalidateCoreVerdictAfterBodyChange(before)
	if jsonOrigin {
		o := state.Origin{}
		if node.Origin != nil {
			o = *node.Origin
		}
		o.Kind = state.OriginKindJSON
		o.Raw = mat.OriginRaw
		node.Origin = &o
	}
	if err := s.Save(statePath); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	debuglog.InfoLog("tailscale: exit node choice saved to the node body")
	if ac.StateService != nil {
		ac.StateService.MarkConfigStale()
	}
	return ac.RebuildConfigIfDirty()
}
