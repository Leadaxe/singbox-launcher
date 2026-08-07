package subscription

import "testing"

// Сырой пробел в userinfo: net/url отвергает такой URI целиком
// ("invalid userinfo"), и нода терялась. Встречается в публичных списках,
// где в логин попадает промо-подпись с пробелом-разделителем.
func TestParseNodeAcceptsSpaceInUserinfo(t *testing.T) {
	uri := "vless://Telegramjoin:TurboConfigs @176.65.151.209:80?encryption=none&type=raw&headerType=http&host=play.google.com&path=%2F&security=none#node"

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
	// Пробел не должен «съесть» часть логина: UUID берётся из userinfo.
	if node.UUID == "" {
		t.Error("uuid must be extracted from userinfo")
	}
}

func TestPercentEncodeUserinfoSpaces(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "space in userinfo is encoded",
			in:   "vless://user name@host:443?x=1",
			want: "vless://user%20name@host:443?x=1",
		},
		{
			name: "multiple spaces are encoded",
			in:   "vless://a b:c d@host:443",
			want: "vless://a%20b:c%20d@host:443",
		},
		{
			name: "no space is left untouched",
			in:   "vless://uuid@host:443?x=1#tag",
			want: "vless://uuid@host:443?x=1#tag",
		},
		{
			name: "no userinfo is left untouched",
			in:   "ss://host:443#tag",
			want: "ss://host:443#tag",
		},
		{
			name: "at-sign inside the fragment is not a userinfo separator",
			in:   "vless://uuid@host:443#name with @handle",
			want: "vless://uuid@host:443#name with @handle",
		},
		{
			name: "at-sign inside the query is not a userinfo separator",
			in:   "vless://host:443?note=a b@c",
			want: "vless://host:443?note=a b@c",
		},
		{
			name: "no scheme separator is left untouched",
			in:   "not a uri at all",
			want: "not a uri at all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentEncodeUserinfoSpaces(tt.in); got != tt.want {
				t.Fatalf("percentEncodeUserinfoSpaces() = %q, want %q", got, tt.want)
			}
		})
	}
}
