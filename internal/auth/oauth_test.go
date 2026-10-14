package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
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

func TestRevokeSendsRefreshToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm.Get("token")
	}))
	defer srv.Close()
	old := revokeURL
	revokeURL = srv.URL
	defer func() { revokeURL = old }()

	c := &Config{configDir: t.TempDir()}
	if err := c.Revoke(context.Background()); err == nil {
		t.Error("expected error with no stored token")
	}
	if err := c.SaveToken(&oauth2.Token{AccessToken: "at", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Revoke(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "rt" {
		t.Errorf("revoked %q, want refresh token", got)
	}
}
