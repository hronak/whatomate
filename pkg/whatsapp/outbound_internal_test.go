package whatsapp

import "testing"

func TestClampRunes(t *testing.T) {
	// Meta measures interactive button limits in characters. The byte slicing
	// this replaced could cut a multi-byte character in half and emit invalid
	// UTF-8, which Meta rejects — and it under-counted, truncating shorter
	// than the limit actually allows.
	for _, tc := range []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"under the limit", "short", 20, "short"},
		{"exactly at the limit", "abcdefgh", 8, "abcdefgh"},
		{"truncated", "truncate me please now", 8, "truncate"},
		{"multi-byte kept whole", "héllo wörld", 5, "héllo"},
		{"non-latin script", "日本語テキスト", 3, "日本語"},
		{"empty", "", 5, ""},
		{"zero limit", "abc", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := clampRunes(tc.in, tc.n)
			if got != tc.want {
				t.Errorf("clampRunes(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
			if len([]rune(got)) > tc.n {
				t.Errorf("result %q is %d runes, over the limit of %d", got, len([]rune(got)), tc.n)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	// Meta sends Retry-After in either RFC 9110 form. Misreading it means
	// either hammering a throttled endpoint or waiting far too long.
	if got := parseRetryAfter("30"); got.Seconds() != 30 {
		t.Errorf("delay-seconds form = %v, want 30s", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("empty header = %v, want 0", got)
	}
	if got := parseRetryAfter("not-a-date"); got != 0 {
		t.Errorf("unparseable header = %v, want 0", got)
	}
	if got := parseRetryAfter("-5"); got != 0 {
		t.Errorf("negative delay = %v, want 0", got)
	}
	// An HTTP-date already in the past yields no wait.
	if got := parseRetryAfter("Mon, 02 Jan 2006 15:04:05 GMT"); got != 0 {
		t.Errorf("past date = %v, want 0", got)
	}
}
