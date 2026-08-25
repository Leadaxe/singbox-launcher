package platform

import "testing"

// Оба реальных формата из лога лаунчера: REST-клиент и gRPC заворачивают
// dial-ошибку по-разному, а адрес нужно достать из обоих.
func TestHostPortFromDialErr(t *testing.T) {
	cases := []struct {
		name, msg, want string
	}{
		{
			"rest",
			`Get "https://192.168.10.1:19091/admin/status": dial tcp 192.168.10.1:19091: connect: no route to host`,
			"192.168.10.1:19091",
		},
		{
			"grpc",
			`lxd remote GetGroups: rpc error: code = Unavailable desc = connection error: desc = "transport: Error while dialing: dial tcp 192.168.10.1:19091: connect: no route to host"`,
			"192.168.10.1:19091",
		},
		{"no dial", "context deadline exceeded", ""},
		{"hostname not ip", "dial tcp router.lan:19091: connect: no route to host", ""},
	}
	for _, c := range cases {
		if got := HostPortFromDialErr(c.msg); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
