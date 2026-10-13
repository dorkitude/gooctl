# gooctl

A simple CLI for Google Workspace (Gmail & Calendar).  Give this to your Claude Code or another CLI agent, and let them manage your life :)



<img width="1536" height="1024" alt="image" src="https://github.com/user-attachments/assets/1a96b8a7-1c59-423f-9a33-48c726074a75" />


## Privacy & Security

**Your data is yours.** When you set up gooctl, you create your own Google OAuth app in your personal (or work) Google Cloud Console. This means:

- All credentials are stored locally on your machine (`~/.config/gooctl/`)
- Your OAuth app belongs to you, not the gooctl developers
- We have zero access to your emails, calendar, or any of your data
- No telemetry, no analytics, no phone-home

## Installation

```bash
go install github.com/dorkitude/gooctl@latest
```

Make sure `~/go/bin` is in your PATH:

```bash
# Add to ~/.zshrc or ~/.bashrc
export PATH="$PATH:$HOME/go/bin"
```

## Setup

Before using gooctl, you need to do a one-time Google OAuth setup (~2 min):

```bash
gooctl auth setup
```

This will walk you through creating a personal OAuth app registration (free, required by Google for API access).

Then login:

```bash
gooctl auth login
```

### Calendar-only and headless logins

Ask Google only for calendar access (no Gmail), and keep each identity in its own directory:

```bash
export GOOCTL_CONFIG_DIR=~/.config/gooctl-calendar   # holds credentials.json, token.json, scopes.json
gooctl auth login --scopes calendar-readonly         # or: calendar (read + edit events), all (default)
```

On a machine without a browser (a VM), print the URL to a file, open it on any computer, and
paste back the URL the browser lands on (a localhost page that won't load):

```bash
gooctl auth login --scopes calendar --no-browser --url-file /tmp/gooctl-login-url \
  --login-hint you@example.com
```

The login uses PKCE and a random state. `gooctl auth status` lists the granted scopes.
With a calendar-only login, `mail` commands get a 403 from Google.

## Usage

### Authentication

```bash
gooctl auth login     # Login via browser
gooctl auth status    # Check login status
gooctl auth logout    # Remove stored credentials
```

### Mail

```bash
gooctl mail search "from:someone@example.com"
gooctl mail search "subject:invoice"
gooctl mail search "is:unread after:2024/01/01"
gooctl mail unread                              # List unread emails
gooctl mail read <message-id>                   # Read full email
gooctl mail thread <thread-id>                  # View entire thread
gooctl mail send --to "x@y.com" --subject "Hi" --body "Hello!"
gooctl mail reply <message-id> --body "Thanks!"
```

### Calendar

```bash
gooctl calendar today                           # Today's events
gooctl calendar week                            # This week's events
gooctl calendar upcoming                        # Upcoming events
gooctl calendar search "standup"                # Search events
gooctl calendar show <event-id>                 # Event details
gooctl calendar create --title "Meeting" --start "2024-01-20T10:00:00" --end "2024-01-20T11:00:00"
gooctl calendar create --title "Birthday" --date "2024-01-20" --all-day
gooctl calendar delete <event-id>
```

`--calendar <id>` (or `-c`) works on every calendar command; `--calendar all` reads across
every calendar in `gooctl cal list` (shared and subscribed ones too). `--json` prints JSON
for `list`, `today`, `week`, `upcoming` and `search`.

Shorthand: `gooctl cal` works too!

## Config Location

Credentials and tokens are stored in `~/.config/gooctl/`

## Claude Code Integration

If you use [Claude Code](https://claude.ai/code), add this to your `~/.claude/CLAUDE.md` so Claude knows how to use gooctl:

```markdown
## gooctl (GSuite CLI)

- **Mail:** `gooctl mail search "query"`, `gooctl mail unread`, `gooctl mail read <id>`
- **Calendar:** `gooctl calendar today`, `gooctl cal week`, `gooctl cal search "query"`
- **Calendar mgmt:** `gooctl cal create --title "X" --start "..." --end "..."`
- **Auth:** `gooctl auth login`, `gooctl auth status`
```

### Superhuman Users

If you use Superhuman, it syncs AI labels to Gmail that gooctl can query:

```bash
# Emails needing your response (filtered by Superhuman AI)
gooctl mail search 'label:[Superhuman]/AI/Respond'

# Filter out spam/marketing
gooctl mail search 'is:inbox -label:[Superhuman]/AI/Pitch -label:[Superhuman]/AI/Marketing'

# List all labels
gooctl mail labels
```
