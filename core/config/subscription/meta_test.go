package subscription

import (
	"net/http"
	"singbox-launcher/core/state"
	"testing"
)

// TestParseHeaders_V2BoardLike — типичный V2Board / Xboard response.
func TestParseHeaders_V2BoardLike(t *testing.T) {
	h := http.Header{}
	h.Set("Subscription-Userinfo", "upload=12345; download=67890; total=53687091200; expire=1717171717")
	h.Set("Profile-Title", "My VPN")
	h.Set("Profile-Update-Interval", "24")
	h.Set("Support-Url", "https://support.example.com/")
	h.Set("Profile-Web-Page-Url", "https://example.com/profile")
	h.Set("Content-Disposition", `attachment; filename="my profile.txt"`)

	m := ParseHeaders(h)

	if m.UserInfo == nil {
		t.Fatalf("UserInfo nil")
	}
	if m.UserInfo.UploadBytes != 12345 || m.UserInfo.DownloadBytes != 67890 {
		t.Errorf("upload/download: %+v", m.UserInfo)
	}
	if m.UserInfo.TotalBytes != 53687091200 || m.UserInfo.ExpireUnix != 1717171717 {
		t.Errorf("total/expire: %+v", m.UserInfo)
	}
	if m.ProfileTitle != "My VPN" {
		t.Errorf("ProfileTitle = %q", m.ProfileTitle)
	}
	if m.ProfileUpdateIntervalHours != 24 {
		t.Errorf("ProfileUpdateIntervalHours = %d", m.ProfileUpdateIntervalHours)
	}
	if m.SupportURL != "https://support.example.com/" {
		t.Errorf("SupportURL = %q", m.SupportURL)
	}
	if m.ProfileWebPageURL != "https://example.com/profile" {
		t.Errorf("ProfileWebPageURL = %q", m.ProfileWebPageURL)
	}
	if m.ContentDispositionFilename != "my profile.txt" {
		t.Errorf("ContentDispositionFilename = %q", m.ContentDispositionFilename)
	}
}

// TestParseHeaders_NilAndEmpty — устойчивость к пустому/nil header'у.
func TestParseHeaders_NilAndEmpty(t *testing.T) {
	if got := ParseHeaders(nil); !isEmptyMeta(got) {
		t.Errorf("nil → %+v, want empty", got)
	}
	if got := ParseHeaders(http.Header{}); !isEmptyMeta(got) {
		t.Errorf("empty header → %+v, want empty", got)
	}
}

// TestParseHeaders_MalformedUserInfo — кривой формат не паникует, не
// возвращает мусорный UserInfo.
func TestParseHeaders_MalformedUserInfo(t *testing.T) {
	h := http.Header{}
	h.Set("Subscription-Userinfo", "this is garbage")
	m := ParseHeaders(h)
	if m.UserInfo != nil {
		t.Errorf("expected nil UserInfo, got %+v", m.UserInfo)
	}
}

// TestParseInlineComments_SameAsHeaders — те же значения, что в HTTP-варианте,
// но эмитятся как `#header: value` в первой строке.
func TestParseInlineComments_SameAsHeaders(t *testing.T) {
	body := []byte(
		"#subscription-userinfo: upload=12345; download=67890; total=53687091200; expire=1717171717\n" +
			"#profile-title: My VPN\n" +
			"#profile-update-interval: 24\n" +
			"#support-url: https://support.example.com/\n" +
			"#profile-web-page-url: https://example.com/profile\n" +
			"#content-disposition: attachment; filename=\"my profile.txt\"\n" +
			"vless://uuid@host:443#tokyo\n" +
			"vless://uuid@host:443#fra\n",
	)

	m := ParseInlineComments(body)
	if m.UserInfo == nil || m.UserInfo.TotalBytes != 53687091200 {
		t.Errorf("UserInfo: %+v", m.UserInfo)
	}
	if m.ProfileTitle != "My VPN" {
		t.Errorf("ProfileTitle = %q", m.ProfileTitle)
	}
	if m.ProfileUpdateIntervalHours != 24 {
		t.Errorf("ProfileUpdateIntervalHours = %d", m.ProfileUpdateIntervalHours)
	}
	if m.SupportURL != "https://support.example.com/" {
		t.Errorf("SupportURL = %q", m.SupportURL)
	}
	if m.ProfileWebPageURL != "https://example.com/profile" {
		t.Errorf("ProfileWebPageURL = %q", m.ProfileWebPageURL)
	}
	if m.ContentDispositionFilename != "my profile.txt" {
		t.Errorf("ContentDispositionFilename = %q", m.ContentDispositionFilename)
	}
}

// TestParseInlineComments_StopsOnFirstNonComment — после первой
// not-#-prefixed строки парсер прекращает сканить.
func TestParseInlineComments_StopsOnFirstNonComment(t *testing.T) {
	body := []byte(
		"#profile-title: First\n" +
			"vless://nodes\n" +
			"#profile-title: Second\n", // должно быть проигнорировано
	)
	m := ParseInlineComments(body)
	if m.ProfileTitle != "First" {
		t.Errorf("ProfileTitle = %q, want First", m.ProfileTitle)
	}
}

// TestParseInlineComments_SkipsBlankLines — пустые строки в начале не
// останавливают сканирование.
func TestParseInlineComments_SkipsBlankLines(t *testing.T) {
	body := []byte("\n\n#profile-title: Hello\nvless://node\n")
	m := ParseInlineComments(body)
	if m.ProfileTitle != "Hello" {
		t.Errorf("ProfileTitle = %q, want Hello", m.ProfileTitle)
	}
}

// TestParseInlineComments_Empty — пустое тело не паникует.
func TestParseInlineComments_Empty(t *testing.T) {
	if got := ParseInlineComments(nil); !isEmptyMeta(got) {
		t.Errorf("nil body: %+v", got)
	}
	if got := ParseInlineComments([]byte{}); !isEmptyMeta(got) {
		t.Errorf("empty body: %+v", got)
	}
}

// TestMergeMeta_HeadersWin — HTTP headers приоритетнее inline.
func TestMergeMeta_HeadersWin(t *testing.T) {
	headers := state.SubscriptionMeta{
		ProfileTitle: "FromHeader",
	}
	inline := state.SubscriptionMeta{
		ProfileTitle: "FromInline",
		SupportURL:   "https://inline.example/",
	}
	got := MergeMeta(headers, inline)
	if got.ProfileTitle != "FromHeader" {
		t.Errorf("ProfileTitle merge: got %q, want FromHeader", got.ProfileTitle)
	}
	if got.SupportURL != "https://inline.example/" {
		t.Errorf("SupportURL fallback: got %q", got.SupportURL)
	}
}

// TestMergeMeta_UserInfoFallback — UserInfo берётся из inline, если
// в headers nil.
func TestMergeMeta_UserInfoFallback(t *testing.T) {
	headers := state.SubscriptionMeta{}
	inline := state.SubscriptionMeta{
		UserInfo: &state.UserInfo{TotalBytes: 100},
	}
	got := MergeMeta(headers, inline)
	if got.UserInfo == nil || got.UserInfo.TotalBytes != 100 {
		t.Errorf("UserInfo fallback failed: %+v", got.UserInfo)
	}
}

// TestParseSubscriptionUserinfo_Various — формат-тестинг.
func TestParseSubscriptionUserinfo_Various(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want *state.UserInfo
	}{
		{
			"all fields semicolon",
			"upload=1; download=2; total=3; expire=4",
			&state.UserInfo{UploadBytes: 1, DownloadBytes: 2, TotalBytes: 3, ExpireUnix: 4},
		},
		{
			"all fields comma",
			"upload=1,download=2,total=3,expire=4",
			&state.UserInfo{UploadBytes: 1, DownloadBytes: 2, TotalBytes: 3, ExpireUnix: 4},
		},
		{
			"partial",
			"total=999",
			&state.UserInfo{TotalBytes: 999},
		},
		{
			"extra whitespace",
			"  upload=1  ;  download=2  ",
			&state.UserInfo{UploadBytes: 1, DownloadBytes: 2},
		},
		{
			"unknown keys ignored",
			"upload=1; foobar=999",
			&state.UserInfo{UploadBytes: 1},
		},
		{
			"all garbage",
			"hello world",
			nil,
		},
		{
			"empty",
			"",
			nil,
		},
	}
	for _, c := range cases {
		got := parseSubscriptionUserinfo(c.in)
		if (got == nil) != (c.want == nil) {
			t.Errorf("%s: nil mismatch (got %+v, want %+v)", c.name, got, c.want)
			continue
		}
		if got != nil && *got != *c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestDecodeProfileTitle_Base64Variants — base64 detection / fallback.
func TestDecodeProfileTitle_Base64Variants(t *testing.T) {
	// "Hello мир" → utf-8 bytes → standard base64
	plainUTF8 := "Hello мир"
	encoded := "SGVsbG8g0LzQuNGA"

	cases := map[string]string{
		"plain ASCII":          "My VPN",
		"plain UTF-8":          plainUTF8,
		"base64 prefix":        "base64:" + encoded,
		"raw base64 no prefix": encoded,
		"garbage that looks like base64 but decodes to junk": "ABCD", // короткое, decode='\x00\x10\x83'
	}

	wantTitle := map[string]string{
		"plain ASCII":          "My VPN",
		"plain UTF-8":          plainUTF8,
		"base64 prefix":        plainUTF8,
		"raw base64 no prefix": plainUTF8,
		// "ABCD" decodes to bytes that are control chars → fallback на оригинал.
		"garbage that looks like base64 but decodes to junk": "ABCD",
	}

	for name, in := range cases {
		got := decodeProfileTitle(in)
		if got != wantTitle[name] {
			t.Errorf("%s: decodeProfileTitle(%q) = %q, want %q", name, in, got, wantTitle[name])
		}
	}
}

// TestParseContentDispositionFilename_Variants — quoted, raw, RFC5987.
func TestParseContentDispositionFilename_Variants(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="my profile.txt"`:         "my profile.txt",
		`attachment; filename=plain.txt`:                "plain.txt",
		`attachment; filename*=UTF-8''My%20Profile.txt`: "My Profile.txt",
		``:        "",
		`garbage`: "",
	}
	for in, want := range cases {
		got := parseContentDispositionFilename(in)
		if got != want {
			t.Errorf("parseContentDispositionFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

// isEmptyMeta — true если все поля meta пусты.
func isEmptyMeta(m state.SubscriptionMeta) bool {
	return m.UserInfo == nil &&
		m.ProfileTitle == "" &&
		m.ProfileUpdateIntervalHours == 0 &&
		m.SupportURL == "" &&
		m.ProfileWebPageURL == "" &&
		m.ContentDispositionFilename == "" &&
		m.URLAtFetch == "" &&
		m.LastFetchedAt == "" &&
		m.LastStatus == "" &&
		m.ErrorCount == 0 &&
		m.LastErrorMsg == "" &&
		m.HTTPStatusCode == 0 &&
		m.RawBodyBytes == 0 &&
		m.NodesCountFetched == 0 &&
		!m.Truncated &&
		len(m.PreviewNodes) == 0
}
