package textnorm

import "testing"

func TestStripANSI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain", "plain"},
		{"\x1b[31mFATAL\x1b[0m: boom", "FATAL: boom"},
		{"[31mFATAL[0m: boom", "FATAL: boom"},
		{"line1\n\x1b[1;31merr\x1b[0m\nline3", "line1\nerr\nline3"},
	}
	for _, tc := range cases {
		if got := StripANSI(tc.in); got != tc.want {
			t.Errorf("StripANSI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
