// File source_body_edit_test.go — правка тела server-узла из окна источника
// (SPEC 118 Т8, поведенческие проверки W6).
//
// Единственное, чего нельзя допустить у обеих кнопок вкладки JSON, — испортить
// рабочий узел неудачной попыткой: ошибка обязана быть ОТКАТОМ, а не
// полупринятой правкой. До SPEC 118 «Reset to URI» переписывал ConfigJSON
// пустотой, и узел с неразбираемым URI терял тело, которое до этого работало.
package tabs

import (
	"encoding/json"
	"strings"
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

const workingBody = `{"type":"trojan","server":"a.example","server_port":443,"password":"p"}`

func serverWithBody(originKind, originRaw string) *wizardmodels.Source {
	return &wizardmodels.Source{Node: wizardmodels.Node{
		Kind:    wizardmodels.SourceKindServer,
		Tag:     "srv",
		Enabled: true,
		Body:    json.RawMessage(workingBody),
		Origin:  &wizardmodels.Origin{Kind: originKind, Raw: originRaw},
	}}
}

// Regen на неразбираемом raw — откат: тело и происхождение те же.
func TestRegenFromRawRollsBackOnBrokenRaw(t *testing.T) {
	src := serverWithBody(wizardmodels.OriginKindURI, "vless://not-a-valid-uri")

	if err := regenServerBodyFromRaw(&src.Node); err == nil {
		t.Fatal("неразбираемый raw принят без ошибки — узел молча испорчен")
	}
	if string(src.Body) != workingBody {
		t.Errorf("тело изменилось при откате: %s", src.Body)
	}
	if src.Origin == nil || src.Origin.Raw != "vless://not-a-valid-uri" {
		t.Errorf("происхождение изменилось при откате: %+v", src.Origin)
	}
}

// Regen на рабочем URI пересобирает тело — и делает это ТЕМ ЖЕ путём, что
// fetch и миграция: тело валидный sing-box outbound с типом.
func TestRegenFromRawRebuildsBody(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@nl.example:443?security=tls&sni=nl.example#nl"
	src := serverWithBody(wizardmodels.OriginKindURI, uri)

	if err := regenServerBodyFromRaw(&src.Node); err != nil {
		t.Fatalf("рабочий URI не пересобрался: %v", err)
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(src.Body, &ob); err != nil {
		t.Fatalf("тело после Regen не JSON: %v", err)
	}
	if ob["type"] != "vless" {
		t.Errorf("тип тела = %v, ожидали vless", ob["type"])
	}
	if ob["server"] != "nl.example" {
		t.Errorf("сервер = %v, ожидали nl.example", ob["server"])
	}
	// Тег и detour — собственность модели: в теле их быть не должно, иначе
	// сборка проштамповала бы их поверх и получила два источника правды.
	if _, has := ob["detour"]; has {
		t.Error("тело несёт detour — он собственность модели, не тела")
	}
	// Происхождение остаётся URI: Regen не переписывает то, ИЗ ЧЕГО узел.
	if src.Origin == nil || src.Origin.Raw != uri {
		t.Errorf("Regen переписал происхождение: %+v", src.Origin)
	}
}

// Regen без происхождения — ошибка, а не пустое тело.
func TestRegenFromRawWithoutOrigin(t *testing.T) {
	src := &wizardmodels.Source{Node: wizardmodels.Node{
		Kind: wizardmodels.SourceKindServer, Body: json.RawMessage(workingBody)}}
	if err := regenServerBodyFromRaw(&src.Node); err == nil {
		t.Fatal("узел без origin пересобрался из ниоткуда")
	}
	if string(src.Body) != workingBody {
		t.Errorf("тело изменилось: %s", src.Body)
	}
}

// Apply на битом вводе — откат; на валидном — тело принято, а происхождение
// становится JSON-овым (узел собран руками, а не из URI).
func TestApplyServerBodyJSON(t *testing.T) {
	t.Run("битый JSON откатывается", func(t *testing.T) {
		src := serverWithBody(wizardmodels.OriginKindURI, "vless://x")
		if err := applyServerBodyJSON(&src.Node, "{not json"); err == nil {
			t.Fatal("битый JSON принят")
		}
		if string(src.Body) != workingBody {
			t.Errorf("тело изменилось при откате: %s", src.Body)
		}
	})

	t.Run("объект без type откатывается", func(t *testing.T) {
		src := serverWithBody(wizardmodels.OriginKindURI, "vless://x")
		if err := applyServerBodyJSON(&src.Node, `{"server":"a.example"}`); err == nil {
			t.Fatal("outbound без type принят — ядро на нём не стартует")
		}
		if string(src.Body) != workingBody {
			t.Errorf("тело изменилось при откате: %s", src.Body)
		}
	})

	t.Run("валидный объект принят, порядок ключей сохранён", func(t *testing.T) {
		src := serverWithBody(wizardmodels.OriginKindURI, "vless://x")
		const edited = `{"type":"trojan","server":"b.example","server_port":8443,"password":"q"}`
		if err := applyServerBodyJSON(&src.Node, edited); err != nil {
			t.Fatalf("валидный объект отвергнут: %v", err)
		}
		// Порядок ключей — это то, что написал пользователь: пересортировка
		// меняла бы тело на каждом сохранении.
		if !strings.HasPrefix(string(src.Body), `{"type":"trojan","server":"b.example"`) {
			t.Errorf("порядок ключей не сохранён: %s", src.Body)
		}
		// Происхождение ПЕРЕЖИВАЕТ правку тела: правка тела — это правка
		// тела, а не смена того, откуда узел взялся. Иначе узел из wg-quick
		// INI после первой же правки JSON терял исходник со всеми
		// комментариями, и «Regen from raw» пересобирал его уже из нашего
		// собственного вывода.
		if src.Origin == nil || src.Origin.Kind != wizardmodels.OriginKindURI {
			t.Errorf("происхождение = %+v, ожидали прежний uri — правка тела его не меняет", src.Origin)
		}
		if src.Origin != nil && src.Origin.Raw != "vless://x" {
			t.Errorf("исходник переписан: %q", src.Origin.Raw)
		}
	})
}
