package subscription

import "testing"

// Сырой пробел в userinfo: net/url отвергает такой URI целиком
// ("invalid userinfo"), и нода терялась. Встречается в публичных списках,
// где в логин попадает промо-подпись с пробелом-разделителем.
//
// Транспорт у живой ссылки был `type=raw&headerType=http`, и с контракта
// 1.1.52 такой узел отбраковывается целиком (обфускация Xray заголовком у
// ядра пары не имеет). Предмет ЭТОГО теста — пробел в userinfo, а не
// транспорт, поэтому ссылка переведена на ws: иначе тест проверял бы
// отбраковку вместо того, ради чего он написан.
func TestParseNodeAcceptsSpaceInUserinfo(t *testing.T) {
	uri := "vless://Telegramjoin:TurboConfigs @176.65.151.209:80?encryption=none&type=ws&host=play.google.com&path=%2F&security=none#node"

	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode() error: %v", err)
	}
	if node == nil {
		t.Fatal("ParseNode() returned nil node")
	}
	if node.Server != "176.65.151.209" {
		t.Errorf("server = %q, want 176.65.151.209", node.Server)
	}
	if node.Port != 80 {
		t.Errorf("port = %d, want 80", node.Port)
	}
	// Пробел не должен «съесть» часть логина: uuid берётся из userinfo.
	if uuid, _ := node.Outbound["uuid"].(string); uuid == "" {
		t.Error("uuid must be extracted from userinfo")
	}
}
