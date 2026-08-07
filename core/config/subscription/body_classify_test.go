package subscription

import "testing"

// SPEC 094 A1 — классификатор формата тела подписки.
func TestClassifySubscriptionBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want BodyKind
	}{
		{
			name: "empty body is uri list",
			body: "",
			want: BodyKindURIList,
		},
		{
			name: "whitespace only is uri list",
			body: "   \n\t  ",
			want: BodyKindURIList,
		},
		{
			name: "plain uri list",
			body: "vless://uuid@example.com:443?security=tls#node\nss://abc@1.2.3.4:8388#other",
			want: BodyKindURIList,
		},
		{
			name: "single singbox outbound",
			body: `{"type":"vless","tag":"a","server":"example.com","server_port":443,"uuid":"u"}`,
			want: BodyKindSingboxOutbound,
		},
		{
			name: "single selector is an outbound, not a config (type before outbounds)",
			body: `{"type":"selector","tag":"select","outbounds":["a","b"]}`,
			want: BodyKindSingboxOutbound,
		},
		{
			name: "whole singbox config with outbounds",
			body: `{"log":{"level":"info"},"outbounds":[{"type":"vless","tag":"a","server":"e.com","server_port":443}]}`,
			want: BodyKindSingboxConfig,
		},
		{
			name: "whole singbox config with endpoints only",
			body: `{"endpoints":[{"type":"wireguard","tag":"wg","address":["10.0.0.2/32"]}]}`,
			want: BodyKindSingboxConfig,
		},
		{
			name: "singbox outbound array",
			body: `[{"type":"vless","tag":"a","server":"e.com","server_port":443},{"type":"trojan","tag":"b","server":"f.com","server_port":443}]`,
			want: BodyKindSingboxOutboundArray,
		},
		{
			name: "singbox config array",
			body: `[{"outbounds":[{"type":"vless","tag":"a","server":"e.com","server_port":443}]},{"outbounds":[{"type":"trojan","tag":"b","server":"f.com","server_port":443}]}]`,
			want: BodyKindSingboxConfigArray,
		},
		{
			name: "xray array wins over singbox config array",
			body: `[{"remarks":"n1","outbounds":[{"protocol":"vless","tag":"proxy","settings":{"vnext":[{"address":"e.com","port":443,"users":[{"id":"u"}]}]}}]}]`,
			want: BodyKindXrayArray,
		},
		{
			name: "empty json array falls back to uri list",
			body: `[]`,
			want: BodyKindURIList,
		},
		{
			name: "malformed json object falls back to uri list",
			body: `{"type":`,
			want: BodyKindURIList,
		},
		{
			name: "malformed json array falls back to uri list",
			body: `[{"type":"vless"`,
			want: BodyKindURIList,
		},
		{
			name: "json object without type or outbounds is uri list",
			body: `{"foo":"bar"}`,
			want: BodyKindURIList,
		},
		{
			name: "outbounds present but not an array is uri list",
			body: `{"outbounds":"nope"}`,
			want: BodyKindURIList,
		},
		{
			name: "non-json text is uri list",
			body: "just some text\nwithout links",
			want: BodyKindURIList,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifySubscriptionBody(tt.body)
			if got != tt.want {
				t.Fatalf("ClassifySubscriptionBody() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Классификатор терпим к отступам и переводам строк вокруг тела.
func TestClassifySubscriptionBodyTrimsSurroundingWhitespace(t *testing.T) {
	body := "\n\n  {\"type\":\"trojan\",\"tag\":\"t\",\"server\":\"e.com\",\"server_port\":443}  \n"
	if got := ClassifySubscriptionBody(body); got != BodyKindSingboxOutbound {
		t.Fatalf("ClassifySubscriptionBody() = %v, want %v", got, BodyKindSingboxOutbound)
	}
}

func TestBodyKindIsSingbox(t *testing.T) {
	singbox := []BodyKind{
		BodyKindSingboxOutbound,
		BodyKindSingboxOutboundArray,
		BodyKindSingboxConfig,
		BodyKindSingboxConfigArray,
	}
	for _, k := range singbox {
		if !k.IsSingbox() {
			t.Errorf("%v.IsSingbox() = false, want true", k)
		}
	}

	for _, k := range []BodyKind{BodyKindURIList, BodyKindXrayArray} {
		if k.IsSingbox() {
			t.Errorf("%v.IsSingbox() = true, want false", k)
		}
	}
}
