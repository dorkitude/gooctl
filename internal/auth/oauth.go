package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"
)

const (
	// Default redirect URI for local OAuth flow
	redirectURI = "http://localhost:8085/callback"
	// Token file name
	tokenFile = "token.json"
	// Credentials file name
	credentialsFile = "credentials.json"
	// Scopes granted at login
	scopesFile = "scopes.json"
)

// ScopePresets are the named scope sets accepted by `gooctl auth login --scopes`.
var ScopePresets = map[string][]string{
	// Everything gooctl can do (the default).
	"all": {
		gmail.GmailReadonlyScope,
		gmail.GmailSendScope,
		gmail.GmailModifyScope,
		calendar.CalendarReadonlyScope,
		calendar.CalendarScope,
	},
	// Read every calendar the account can see. No Gmail.
	"calendar-readonly": {calendar.CalendarReadonlyScope},
	// Read every calendar and create/edit/delete events. No Gmail.
	"calendar": {calendar.CalendarReadonlyScope, calendar.CalendarEventsScope},
}

// ResolveScopes turns a preset name or a comma-separated list of scope URLs into scopes.
func ResolveScopes(spec string) ([]string, error) {
	if spec == "" {
		spec = "all"
	}
	if preset, ok := ScopePresets[spec]; ok {
		return preset, nil
	}
	var scopes []string
	for _, s := range strings.Split(spec, ",") {
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, "https://www.googleapis.com/auth/") {
			return nil, fmt.Errorf("unknown scope %q: use all, calendar-readonly, calendar, or full scope URLs", s)
		}
		scopes = append(scopes, s)
	}
	return scopes, nil
}

// Config holds the OAuth2 configuration
type Config struct {
	configDir string
	oauth     *oauth2.Config
}

// NewConfig creates a new OAuth2 config
func NewConfig() (*Config, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Try to load credentials from file
	credPath := filepath.Join(configDir, credentialsFile)
	credData, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("credentials file not found at %s - please download OAuth credentials from Google Cloud Console and save them there", credPath)
	}

	// Scopes only matter at login; a stored token keeps what it was granted.
	scopes := ScopePresets["all"]
	if data, err := os.ReadFile(filepath.Join(configDir, scopesFile)); err == nil {
		var saved []string
		if json.Unmarshal(data, &saved) == nil && len(saved) > 0 {
			scopes = saved
		}
	}

	config, err := google.ConfigFromJSON(credData, scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	// Override redirect URI for local flow
	config.RedirectURL = redirectURI

	return &Config{
		configDir: configDir,
		oauth:     config,
	}, nil
}

// getConfigDir returns the configuration directory path.
// GOOCTL_CONFIG_DIR overrides it, so several identities can live side by side.
func getConfigDir() (string, error) {
	if dir := os.Getenv("GOOCTL_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", "gooctl"), nil
}

// GetConfigDir returns the configuration directory (public accessor)
func GetConfigDir() (string, error) {
	return getConfigDir()
}

// TokenPath returns the path to the token file
func (c *Config) TokenPath() string {
	return filepath.Join(c.configDir, tokenFile)
}

// HasValidToken checks if a usable token exists (a refresh token, or an unexpired access token)
func (c *Config) HasValidToken() bool {
	token, err := c.LoadToken()
	if err != nil {
		return false
	}
	return token.RefreshToken != "" || token.Valid()
}

// SetScopes sets the scopes requested at the next login.
func (c *Config) SetScopes(scopes []string) {
	c.oauth.Scopes = scopes
}

// Scopes returns the scopes recorded at the last login.
func (c *Config) Scopes() []string {
	data, err := os.ReadFile(filepath.Join(c.configDir, scopesFile))
	if err != nil {
		return nil
	}
	var scopes []string
	json.Unmarshal(data, &scopes)
	return scopes
}

func (c *Config) saveScopes(token *oauth2.Token) error {
	// Record what Google actually granted, falling back to what was asked for.
	scopes := c.oauth.Scopes
	if granted, ok := token.Extra("scope").(string); ok && granted != "" {
		scopes = strings.Fields(granted)
	}
	data, err := json.MarshalIndent(scopes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.configDir, scopesFile), data, 0600)
}

// LoginOptions controls how Authenticate gets the authorization code.
type LoginOptions struct {
	// NoBrowser prints the URL and reads the redirect URL (or the code) from Input
	// instead of opening a browser and listening on localhost. For headless machines.
	NoBrowser bool
	// URLFile, if set, also writes the auth URL there (0600), so it can be fetched
	// without copying a long URL out of a terminal.
	URLFile string
	// LoginHint pre-selects the Google account.
	LoginHint string
	Input     io.Reader
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// codeFromPaste accepts either the full redirect URL or the bare code.
func codeFromPaste(pasted, state string) (string, error) {
	pasted = strings.TrimSpace(pasted)
	if !strings.Contains(pasted, "://") {
		if pasted == "" {
			return "", fmt.Errorf("nothing pasted")
		}
		return pasted, nil
	}
	u, err := url.Parse(pasted)
	if err != nil {
		return "", fmt.Errorf("could not parse pasted URL: %w", err)
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("Google returned error: %s", e)
	}
	if q.Get("state") != state {
		return "", fmt.Errorf("state mismatch: the URL is from a different login attempt")
	}
	code := q.Get("code")
	if code == "" {
		return "", fmt.Errorf("no code in pasted URL")
	}
	return code, nil
}

// exchangeAndSave trades the code for a token and stores it with its scopes.
func (c *Config) exchangeAndSave(ctx context.Context, code, verifier string) error {
	token, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return fmt.Errorf("failed to exchange code for token: %w", err)
	}
	if err := c.SaveToken(token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}
	return c.saveScopes(token)
}

// LoadToken loads the token from disk
func (c *Config) LoadToken() (*oauth2.Token, error) {
	data, err := os.ReadFile(c.TokenPath())
	if err != nil {
		return nil, err
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// SaveToken saves the token to disk
func (c *Config) SaveToken(token *oauth2.Token) error {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.TokenPath(), data, 0600)
}

// GetClient returns an authenticated HTTP client
func (c *Config) GetClient(ctx context.Context) (*http.Client, error) {
	token, err := c.LoadToken()
	if err != nil {
		return nil, fmt.Errorf("not authenticated - run 'gooctl auth' first")
	}

	// Create token source that auto-refreshes
	tokenSource := c.oauth.TokenSource(ctx, token)

	// Get potentially refreshed token
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Save refreshed token if it changed
	if newToken.AccessToken != token.AccessToken {
		if err := c.SaveToken(newToken); err != nil {
			// Log but don't fail
			fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed token: %v\n", err)
		}
	}

	return oauth2.NewClient(ctx, tokenSource), nil
}

// Authenticate performs the OAuth2 flow (PKCE + random state)
func (c *Config) Authenticate(ctx context.Context, opts LoginOptions) error {
	state := randomState()
	verifier := oauth2.GenerateVerifier()
	authOpts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier),
	}
	if opts.LoginHint != "" {
		authOpts = append(authOpts, oauth2.SetAuthURLParam("login_hint", opts.LoginHint))
	}
	authURL := c.oauth.AuthCodeURL(state, authOpts...)

	if opts.URLFile != "" {
		if err := os.WriteFile(opts.URLFile, []byte(authURL+"\n"), 0600); err != nil {
			return fmt.Errorf("failed to write URL file: %w", err)
		}
	}

	if opts.NoBrowser {
		if opts.URLFile != "" {
			fmt.Printf("Auth URL written to %s\n", opts.URLFile)
		} else {
			fmt.Printf("Open this URL in a browser:\n%s\n", authURL)
		}
		fmt.Println("After approving, the browser lands on a localhost page that won't load.")
		fmt.Print("Paste that page's full URL (or just the code) here: ")
		in := opts.Input
		if in == nil {
			in = os.Stdin
		}
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("failed to read pasted URL: %w", err)
		}
		code, err := codeFromPaste(line, state)
		if err != nil {
			return err
		}
		return c.exchangeAndSave(ctx, code, verifier)
	}

	// Create a channel to receive the auth code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start local server to receive callback
	server := &http.Server{Addr: ":8085"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("state mismatch in callback")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			http.Error(w, "No code received", http.StatusBadRequest)
			return
		}

		// Send success page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>gooctl - Authentication Successful</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #4285F4 0%%, #34A853 100%%);
        }
        .card {
            background: white;
            padding: 40px;
            border-radius: 16px;
            box-shadow: 0 4px 20px rgba(0,0,0,0.15);
            text-align: center;
        }
        h1 { color: #34A853; margin-bottom: 10px; }
        p { color: #666; }
    </style>
</head>
<body>
    <div class="card">
        <h1>✓ Authentication Successful!</h1>
        <p>You can close this window and return to your terminal.</p>
    </div>
</body>
</html>`)

		codeChan <- code
	})

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If the browser doesn't open, visit this URL:\n%s\n\n", authURL)

	// Open browser
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}

	// Wait for callback or timeout
	var code string
	select {
	case code = <-codeChan:
		// Success
	case err := <-errChan:
		server.Shutdown(ctx)
		return fmt.Errorf("authentication failed: %w", err)
	case <-time.After(5 * time.Minute):
		server.Shutdown(ctx)
		return fmt.Errorf("authentication timed out")
	case <-ctx.Done():
		server.Shutdown(ctx)
		return ctx.Err()
	}

	// Shutdown server
	server.Shutdown(ctx)

	return c.exchangeAndSave(ctx, code, verifier)
}

// Logout removes the stored token
func (c *Config) Logout() error {
	tokenPath := c.TokenPath()
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		return nil // Already logged out
	}
	os.Remove(filepath.Join(c.configDir, scopesFile))
	return os.Remove(tokenPath)
}

// openBrowser opens the default browser to the specified URL
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}
