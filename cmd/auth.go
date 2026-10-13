package cmd

import (
	"context"
	"fmt"

	"github.com/dorkitude/gooctl/internal/auth"
	"github.com/dorkitude/gooctl/internal/ui"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Google",
	Long:  `Manage authentication with Google Workspace.`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login with Google OAuth",
	Long: `Opens a browser window to authenticate with Google.

--scopes picks what to ask for: all (default; Gmail + Calendar), calendar-readonly,
calendar (read + edit events), or comma-separated scope URLs. On a headless machine use
--no-browser (and --url-file) and paste back the URL the browser lands on.
Set GOOCTL_CONFIG_DIR to keep a separate identity in its own directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		scopeSpec, _ := cmd.Flags().GetString("scopes")
		noBrowser, _ := cmd.Flags().GetBool("no-browser")
		urlFile, _ := cmd.Flags().GetString("url-file")
		loginHint, _ := cmd.Flags().GetString("login-hint")

		scopes, err := auth.ResolveScopes(scopeSpec)
		if err != nil {
			return err
		}

		config, err := auth.NewConfig()
		if err != nil {
			return err
		}
		config.SetScopes(scopes)

		// Check if already authenticated
		if config.HasValidToken() {
			fmt.Println(ui.Warning_("Already authenticated. Use 'gooctl auth logout' first to re-authenticate."))
			return nil
		}

		fmt.Println(ui.TitleStyle.Render("🔐 Google Authentication"))
		fmt.Println()

		opts := auth.LoginOptions{NoBrowser: noBrowser, URLFile: urlFile, LoginHint: loginHint}
		if err := config.Authenticate(ctx, opts); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		fmt.Println()
		fmt.Println(ui.Success("Successfully authenticated with Google!"))
		fmt.Println(ui.SubtleStyle.Render("Token saved to: " + config.TokenPath()))

		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	Long:  `Removes locally stored OAuth tokens.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := auth.NewConfig()
		if err != nil {
			return err
		}

		if err := config.Logout(); err != nil {
			return fmt.Errorf("failed to logout: %w", err)
		}

		fmt.Println(ui.Success("Successfully logged out."))
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check authentication status",
	Long:  `Check if you are currently authenticated with Google.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := auth.NewConfig()
		if err != nil {
			return err
		}

		if config.HasValidToken() {
			fmt.Println(ui.Success("Authenticated"))
			fmt.Println(ui.SubtleStyle.Render("Token location: " + config.TokenPath()))
			for _, s := range config.Scopes() {
				fmt.Println(ui.SubtleStyle.Render("Scope: " + s))
			}
		} else {
			fmt.Println(ui.Warning_("Not authenticated"))
			fmt.Println(ui.SubtleStyle.Render("Run 'gooctl auth login' to authenticate"))
		}

		return nil
	},
}

var authSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "One-time setup to connect gooctl to your Google account",
	Long:  `Quick one-time setup to allow gooctl to access your Gmail and Calendar.`,
	Run: func(cmd *cobra.Command, args []string) {
		configDir, _ := auth.GetConfigDir()

		fmt.Println(ui.TitleStyle.Render("🔧 One-Time Setup (takes ~2 min)"))
		fmt.Println()
		fmt.Println(ui.SubtleStyle.Render("Google requires apps to register before accessing your data."))
		fmt.Println(ui.SubtleStyle.Render("You'll create a personal 'app registration' for gooctl. It's free."))
		fmt.Println()
		fmt.Println(ui.SuccessStyle.Render("1.") + " Open: https://console.cloud.google.com/projectcreate")
		fmt.Println("   → Name it anything (e.g., 'gooctl') → Create")
		fmt.Println()
		fmt.Println(ui.SuccessStyle.Render("2.") + " Enable APIs (click each link while your project is selected):")
		fmt.Println("   → https://console.cloud.google.com/apis/library/gmail.googleapis.com")
		fmt.Println("   → https://console.cloud.google.com/apis/library/calendar-json.googleapis.com")
		fmt.Println()
		fmt.Println(ui.SuccessStyle.Render("3.") + " Set up consent: https://console.cloud.google.com/apis/credentials/consent")
		fmt.Println("   → External → Create → Fill in app name & your email → Save")
		fmt.Println("   → Add yourself as a test user")
		fmt.Println()
		fmt.Println(ui.SuccessStyle.Render("4.") + " Create credentials: https://console.cloud.google.com/apis/credentials")
		fmt.Println("   → Create Credentials → OAuth client ID → Desktop app → Create")
		fmt.Println("   → Copy the Client ID and Client Secret shown")
		fmt.Println()
		fmt.Println(ui.SuccessStyle.Render("5.") + " Create this file: " + configDir + "/credentials.json")
		fmt.Println()
		fmt.Println(`   {
     "installed": {
       "client_id": "YOUR_CLIENT_ID",
       "client_secret": "YOUR_CLIENT_SECRET",
       "auth_uri": "https://accounts.google.com/o/oauth2/auth",
       "token_uri": "https://oauth2.googleapis.com/token",
       "redirect_uris": ["http://localhost"]
     }
   }`)
		fmt.Println()
		fmt.Println()
		fmt.Println(ui.SuccessStyle.Render("6.") + " Run: gooctl auth login")
		fmt.Println()
		fmt.Println(ui.SubtleStyle.Render("This is a one-time thing - after setup, just use gooctl normally! 🎉"))
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authLoginCmd.Flags().String("scopes", "all", "all | calendar-readonly | calendar | comma-separated scope URLs")
	authLoginCmd.Flags().Bool("no-browser", false, "Headless: print the URL and paste back the redirect URL")
	authLoginCmd.Flags().String("url-file", "", "Also write the auth URL to this file (0600)")
	authLoginCmd.Flags().String("login-hint", "", "Google account to pre-select (email)")
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authSetupCmd)
}
