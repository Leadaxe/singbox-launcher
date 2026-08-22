package subscription

import "testing"

// SPEC 103, фаза 2: коды деградации на узле.
//
// Проверяется не «warning залогирован», а «узел помечен кодом»: код уходит в
// конверт корпуса и сверяется между приложениями, лог — нет.

func TestNodeWarningsSetOnDegradation(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "мусорный uTLS-отпечаток заменён каноническим",
			uri:  "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=tls&fp=garbage&sni=example-1.com",
			want: WarnUTLSFingerprintUnknown,
		},
		{
			name: "reality sid нечётной длины снят целиком",
			uri:  "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&pbk=AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw&sid=abc&sni=example-1.com",
			want: WarnRealityShortIDInvalid,
		},
		{
			name: "hysteria2 obfs без пароля снята",
			uri:  "hysteria2://pass123@example-2.com:443?obfs=salamander&sni=example-2.com",
			want: WarnObfsPasswordMissing,
		},
		{
			name: "hysteria2 obfs вне словаря ядра",
			uri:  "hysteria2://pass123@example-2.com:443?obfs=nonsense&obfs-password=p&sni=example-2.com",
			want: WarnObfsUnknown,
		},
		{
			name: "TUIC congestion_control вне словаря",
			uri:  "tuic://11111111-1111-1111-1111-111111111111:pass@example-3.com:443?congestion_control=nonsense&sni=example-3.com",
			want: WarnTuicCongestionInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ParseNode(tc.uri, nil)
			if err != nil {
				t.Fatalf("ParseNode: %v", err)
			}
			if node == nil {
				t.Fatal("узел не разобран")
			}
			if !hasWarning(node.Warnings, tc.want) {
				t.Fatalf("код %q не проставлен; получено: %v", tc.want, node.Warnings)
			}
		})
	}
}

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

// Приведение регистра hex — не деградация: sing-box декодирует ABCD и abcd
// одинаково, и код на такой ноде был бы шумом (ловилось на корпусе: 12
// ложных срабатываний из 12).
func TestRealitySIDCaseIsNotDegradation(t *testing.T) {
	node, err := ParseNode(
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&pbk=AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw&sid=ABCD&sni=example-1.com", nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if hasWarning(node.Warnings, WarnRealityShortIDInvalid) {
		t.Errorf("sid=ABCD помечен как испорченный, хотя это лишь регистр: %v", node.Warnings)
	}
}

// WarnSSHUserDefault остаётся в словаре, но на URI-пути недостижим:
// ParseNode отвергает ssh-ссылку с пустым username раньше, чем дело дойдёт
// до подстановки root (node_parser_core.go: «missing userinfo»). Ветка жива
// для sing-box-импорта, где узел приходит уже разобранным; тест на неё
// появится вместе с покрытием того пути.

func hasWarning(list []string, code string) bool {
	for _, c := range list {
		if c == code {
			return true
		}
	}
	return false
}
