package subscription

import (
	"testing"
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

// ssh_user_default объявлен в реестре (warnings.json, protocols/ssh.json),
// но Go-константы у него больше нет: на URI-пути он был недостижим и до
// SPEC 133 — ссылку с пустым userinfo отбивает валидация раньше подстановки
// root, — а рукописная ветка с подстановкой удалена вместе с парсером.
// Сегодня ssh ведёт движок, и `user` у него объявлен required. Дефолт root
// остаётся у sing-box-импорта, где узел приходит уже разобранным; тест на
// него появится вместе с покрытием того пути.
