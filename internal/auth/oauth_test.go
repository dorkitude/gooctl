package auth

import (
	"strings"
	"testing"
)

func TestCodeFromPaste(t *testing.T) {
	const state = "abc123"
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"4/0Axyz\n", "4/0Axyz", false},
		{"http://localhost:8085/callback?state=abc123&code=4/0Axyz&scope=x", "4/0Axyz", false},
		{"http://localhost:8085/callback?state=other&code=4/0Axyz", "", true},
		{"http://localhost:8085/callback?state=abc123&error=access_denied", "", true},
		{"  ", "", true},
	}
	for _, c := range cases {
		got, err := codeFromPaste(c.in, state)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("codeFromPaste(%q) = %q, %v; want %q, err=%v", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func TestResolveScopes(t *testing.T) {
	got, err := ResolveScopes("calendar-readonly")
	if err != nil || len(got) != 1 || got[0] != "https://www.googleapis.com/auth/calendar.readonly" {
		t.Errorf("calendar-readonly = %v, %v", got, err)
	}
	if _, err := ResolveScopes("gmail"); err == nil {
		t.Error("expected error for unknown preset")
	}
	got, err = ResolveScopes("https://www.googleapis.com/auth/calendar.freebusy")
	if err != nil || len(got) != 1 {
		t.Errorf("raw URL = %v, %v", got, err)
	}
}

// No calendar preset may include a Gmail scope.
func TestCalendarPresetsHaveNoGmail(t *testing.T) {
	for _, name := range []string{"calendar", "calendar-readonly"} {
		for _, s := range ScopePresets[name] {
			if strings.Contains(s, "/auth/gmail") {
				t.Errorf("preset %s includes %s", name, s)
			}
		}
	}
}
