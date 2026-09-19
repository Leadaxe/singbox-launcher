package subscription

import "testing"

// Предупреждение «в ссылке приватный ключ» выдаётся по ТЕЛУ узла, и цена
// ошибки здесь несимметрична: ложное «нет» = ключ уезжает в буфер молча.
// Поэтому тест ведёт схемы разом — включая те, где ключа не бывает и диалог
// появляться не должен.
func TestShareURICarriesPrivateKey(t *testing.T) {
	cases := []struct {
		name string
		out  map[string]interface{}
		want bool
	}{
		{
			name: "ssh inline private_key",
			out: map[string]interface{}{
				"type": "ssh", "server": "10.0.0.1", "server_port": 22, "user": "root",
				"private_key": "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
			},
			want: true,
		},
		{
			// Listable: массив с одним ключом — то же самое.
			name: "ssh private_key as list",
			out: map[string]interface{}{
				"type": "ssh", "server": "10.0.0.1", "server_port": 22,
				"private_key": []interface{}{"-----BEGIN KEY-----"},
			},
			want: true,
		},
		{
			// Путь к файлу — в ссылке едет ПУТЬ, ключа там нет.
			name: "ssh private_key_path only",
			out: map[string]interface{}{
				"type": "ssh", "server": "10.0.0.1", "server_port": 22, "user": "deploy",
				"private_key_path": "/home/user/.ssh/id_rsa",
			},
			want: false,
		},
		{
			// Пароль приватным ключом не считается (решение владельца).
			name: "ssh password only",
			out: map[string]interface{}{
				"type": "ssh", "server": "10.0.0.1", "server_port": 22, "password": "s3cret",
			},
			want: false,
		},
		{
			name: "ssh empty private_key string",
			out: map[string]interface{}{
				"type": "ssh", "server": "10.0.0.1", "private_key": "   ",
			},
			want: false,
		},
		{
			name: "wireguard endpoint",
			out: map[string]interface{}{
				"type": "wireguard", "private_key": "RAUTG+IXUH+KW8Ocva7RTpv6y/gdVQIIgh9MeuzeMtU=",
			},
			want: true,
		},
		{
			name: "amneziawg endpoint (type stays wireguard)",
			out: map[string]interface{}{
				"type": "wireguard", "private_key": "aGVsbG8=", "jc": int64(4), "h1": int64(1),
			},
			want: true,
		},
		{
			name: "masque",
			out: map[string]interface{}{
				"type": "masque", "private_key": "ZGVy", "public_key": "cHVi",
			},
			want: true,
		},
		{
			// uuid секретом ссылки в этом смысле не считается.
			name: "vless",
			out: map[string]interface{}{
				"type": "vless", "server": "example.com", "server_port": 443,
				"uuid": "550e8400-e29b-41d4-a716-446655440000",
			},
			want: false,
		},
		{
			name: "trojan password",
			out: map[string]interface{}{
				"type": "trojan", "server": "example.com", "server_port": 443, "password": "p",
			},
			want: false,
		},
		{name: "nil", out: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShareURICarriesPrivateKey(tc.out); got != tc.want {
				t.Fatalf("ShareURICarriesPrivateKey = %v, want %v", got, tc.want)
			}
		})
	}
}

// Эмиттер и парсер ходят парой: ссылка ssh с inline-ключом обязана
// разобраться нашим же ParseNode обратно в тот же ключ. Раньше этот случай
// вовсе не кодировался (ErrShareURINotSupported).
func TestShareURIFromSSH_InlinePrivateKeyRoundTrip(t *testing.T) {
	const key = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNza+A/B==\n-----END OPENSSH PRIVATE KEY-----"
	out := map[string]interface{}{
		"type":                   "ssh",
		"tag":                    "ssh-key",
		"server":                 "git.example.com",
		"server_port":            2222,
		"user":                   "deploy",
		"private_key":            key,
		"private_key_passphrase": "phrase",
	}
	uri, err := ShareURIFromOutbound(out)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	if !ShareURICarriesPrivateKey(out) {
		t.Fatal("предикат обязан признать эту ссылку несущей приватный ключ")
	}
	n, err := ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v uri=%q", err, uri)
	}
	if got, _ := n.Outbound["private_key"].(string); got != key {
		t.Fatalf("private_key round-trip: got %q", got)
	}
	if got, _ := n.Outbound["user"].(string); got != "deploy" {
		t.Fatalf("user round-trip: got %q", got)
	}
	if got, _ := n.Outbound["private_key_passphrase"].(string); got != "phrase" {
		t.Fatalf("passphrase round-trip: got %q", got)
	}
	if n.Server != "git.example.com" || n.Port != 2222 || n.Tag != "ssh-key" {
		t.Fatalf("server/port/tag round-trip: %q/%d/%q", n.Server, n.Port, n.Tag)
	}
	// private_key_path в ссылку не попал: парсер читает его только при
	// пустом private_key, и эмитить оба значило бы описать узел, которого
	// разбор не даст.
	if _, ok := n.Outbound["private_key_path"]; ok {
		t.Fatal("private_key_path не должен появляться при inline-ключе")
	}
}
