package subscription

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 103, фаза 2: коды деградации на узле.
//
// Проверяется не «warning залогирован», а «узел помечен кодом»: код уходит в
// конверт корпуса и сверяется между приложениями, лог — нет.
//
// С SPEC 131 W2d бо́льшую часть кодов ставит САНИТАЙЗЕР по реестру, а не
// парсер: здесь остались только те, которые парсер видит сам (структура
// ссылки), и проверка «здоровый узел не помечен». Сверка полного набора
// кодов на выходе конвейера — core/config/nodeflow_pipeline_test.go и корпус.

// Здоровый узел не должен получать кодов: ложное срабатывание обесценивает
// весь механизм — пользователь перестаёт читать предупреждения.
func TestCleanNodeHasNoWarnings(t *testing.T) {
	clean := []string{
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=tls&fp=chrome&sni=example-1.com",
		"trojan://testpass123@example-2.com:443?sni=example-2.com",
		"hysteria2://pass123@example-2.com:443?obfs=salamander&obfs-password=secret&sni=example-2.com",
	}
	for _, uri := range clean {
		node, err := ParseNode(uri, nil)
		if err != nil || node == nil {
			t.Fatalf("ParseNode(%q): %v", uri, err)
		}
		if len(node.Warnings) != 0 {
			t.Errorf("здоровый узел помечен: %v\n  %s", node.Warnings, uri)
		}
	}
}

// Приведение регистра hex деградацией не считается (ABCD и abcd ядро
// декодирует одинаково) — проверка переехала на выход конвейера вместе с
// самой чисткой: core/config TestPipelineCleanNodeStaysClean.

// WarnSSHUserDefault остаётся в словаре, но на URI-пути недостижим:
// ParseNode отвергает ssh-ссылку с пустым username раньше, чем дело дойдёт
// до подстановки root (node_parser_core.go: «missing userinfo»). Ветка жива
// для sing-box-импорта, где узел приходит уже разобранным; тест на неё
// появится вместе с покрытием того пути.

func hasWarning(list []configtypes.Warning, code string) bool {
	for _, w := range list {
		if w.Code == code {
			return true
		}
	}
	return false
}
