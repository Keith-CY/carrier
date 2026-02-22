package gateway

import "testing"

func TestTelegramPairCommandMatched(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		code string
		want bool
	}{
		{name: "pair exact", raw: "/pair tg-abc123", code: "tg-abc123", want: true},
		{name: "pair mention suffix", raw: "/pair@mybot tg-abc123", code: "tg-abc123", want: true},
		{name: "start accepted", raw: "/start tg-abc123", code: "tg-abc123", want: true},
		{name: "wrong code", raw: "/pair tg-zzz", code: "tg-abc123", want: false},
		{name: "no command", raw: "hello", code: "tg-abc123", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := telegramPairCommandMatched(tc.raw, tc.code)
			if got != tc.want {
				t.Fatalf("telegramPairCommandMatched(%q,%q)=%v want %v", tc.raw, tc.code, got, tc.want)
			}
		})
	}
}
