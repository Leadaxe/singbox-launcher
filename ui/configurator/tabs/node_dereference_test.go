// File node_dereference_test.go — авторазыменование узла при ручной правке
// (SPEC 116 этап 3, W5; дыра Д5, критерий A4).
//
// Проверяем ДАННЫЕ, а не тексты: правка тела/Regen узла с непустым
// origin.subUrl снимает связь с подпиской, а узел без связи не меняется.
// Текст уведомления не тестируем (правило no-ui-format-tests).
package tabs

import (
	"encoding/json"
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

const derefBody = `{"type":"trojan","server":"a.example","server_port":443,"password":"p"}`

func derefSource(subURL string) *wizardmodels.Source {
	return &wizardmodels.Source{Node: wizardmodels.Node{
		Kind: wizardmodels.SourceKindServer,
		Tag:  "nl-1",
		Body: json.RawMessage(derefBody),
		Origin: &wizardmodels.Origin{
			Kind:   wizardmodels.OriginKindURI,
			Raw:    "vless://b831381d-6324-4d53-ad4f-8cda48b30811@nl.example:443?security=tls&sni=nl.example#nl",
			SubURL: subURL,
		},
	}}
}

// A4: у узла, пришедшего из подписки, ручная правка снимает связь.
func TestDereferenceEditedSourceNode_ClearsSubURL(t *testing.T) {
	src := derefSource("https://example.com/sub")

	if !dereferenceEditedSourceNode(src) {
		t.Fatal("разыменование не сработало — узел остался привязан к подписке")
	}
	if src.Origin.SubURL != "" {
		t.Errorf("subUrl не обнулён: %q", src.Origin.SubURL)
	}
	// Raw и kind обязаны уцелеть: узел отвязан от подписки, а не лишён
	// происхождения — «Regen from raw» после этого обязан работать.
	if src.Origin.Raw == "" || src.Origin.Kind != wizardmodels.OriginKindURI {
		t.Errorf("происхождение потеряно вместе со связью: %+v", src.Origin)
	}
}

// A4, вторая половина: узел без связи правкой не меняется — уведомления быть
// не должно, и повод для него не выдумывается.
func TestDereferenceEditedSourceNode_NoopWithoutSubURL(t *testing.T) {
	src := derefSource("")
	if dereferenceEditedSourceNode(src) {
		t.Fatal("узел без subUrl объявлен разыменованным")
	}
}

// Regen пересобирает тело и обязан оставить узел разыменованным: связь с
// подпиской снимается ровно один раз и обратно не возвращается.
func TestRegenThenDereferenceKeepsNodeFree(t *testing.T) {
	src := derefSource("https://example.com/sub")

	if err := regenServerBodyFromRaw(src); err != nil {
		t.Fatalf("рабочий URI не пересобрался: %v", err)
	}
	// Материализация пересаживает Origin целиком — после неё связи уже нет,
	// и разыменование обязано сообщить «нечего снимать», а не выдумать факт.
	if src.Origin.SubURL != "" {
		t.Fatalf("Regen сохранил связь с подпиской: %q", src.Origin.SubURL)
	}
	if dereferenceEditedSourceNode(src) {
		t.Error("повторное разыменование объявило снятой уже снятую связь")
	}
}

// Apply тела: путь «текст → тело» тоже оставляет узел свободным.
func TestApplyBodyLeavesNodeDereferenced(t *testing.T) {
	src := derefSource("https://example.com/sub")

	const edited = `{"type":"trojan","server":"b.example","server_port":8443,"password":"p2"}`
	if err := applyServerBodyJSON(src, edited); err != nil {
		t.Fatalf("правка тела отвергнута: %v", err)
	}
	if src.Origin.SubURL != "" {
		t.Errorf("узел остался привязан к подписке после ручной правки тела: %q", src.Origin.SubURL)
	}
	if dereferenceEditedSourceNode(src) {
		t.Error("повторное разыменование объявило снятой уже снятую связь")
	}
}
