package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dorkitude/gooctl/internal/auth"
	calpkg "github.com/dorkitude/gooctl/internal/calendar"
	"github.com/dorkitude/gooctl/internal/ui"
	"github.com/spf13/cobra"
)

var calendarCmd = &cobra.Command{
	Use:     "calendar",
	Aliases: []string{"cal"},
	Short:   "View and manage calendar events",
	Long:    `Commands for viewing, searching, and managing calendar events.`,
}

var calTodayCmd = &cobra.Command{
	Use:   "today",
	Short: "Show today's events",
	Long:  `Display all events scheduled for today.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		limit, _ := cmd.Flags().GetInt64("limit")

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		events, err := client.Today(ctx, limit)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(events)
		}

		if len(events) == 0 {
			fmt.Println(ui.Success("No events today! 🎉"))
			return nil
		}

		fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("📅 Today - %s", time.Now().Format("Monday, January 2"))))
		fmt.Println()

		for _, event := range events {
			printEventSummary(event)
		}

		return nil
	},
}

var calWeekCmd = &cobra.Command{
	Use:   "week",
	Short: "Show this week's events",
	Long:  `Display all events scheduled for the next 7 days.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		limit, _ := cmd.Flags().GetInt64("limit")

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		events, err := client.Week(ctx, limit)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(events)
		}

		if len(events) == 0 {
			fmt.Println(ui.Success("No events this week! 🎉"))
			return nil
		}

		fmt.Println(ui.TitleStyle.Render("📅 This Week"))
		fmt.Println()

		currentDay := ""
		for _, event := range events {
			day := event.Start.Format("Monday, January 2")
			if day != currentDay {
				if currentDay != "" {
					fmt.Println()
				}
				fmt.Println(ui.SuccessStyle.Render("▸ " + day))
				currentDay = day
			}
			printEventSummary(event)
		}

		return nil
	},
}

var calUpcomingCmd = &cobra.Command{
	Use:   "upcoming",
	Short: "Show upcoming events",
	Long:  `Display upcoming events starting from now.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		limit, _ := cmd.Flags().GetInt64("limit")

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		events, err := client.Upcoming(ctx, limit)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(events)
		}

		if len(events) == 0 {
			fmt.Println(ui.Success("No upcoming events!"))
			return nil
		}

		fmt.Println(ui.TitleStyle.Render("📅 Upcoming Events"))
		fmt.Println()

		for _, event := range events {
			printEventSummary(event)
		}

		return nil
	},
}

var calSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search calendar events",
	Long:  `Search for calendar events matching a query.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		query := strings.Join(args, " ")
		limit, _ := cmd.Flags().GetInt64("limit")
		days, _ := cmd.Flags().GetInt("days")

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		now := time.Now()
		start := now.AddDate(0, 0, -days)
		end := now.AddDate(0, 0, days)

		events, err := client.Search(ctx, query, start, end, limit)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(events)
		}

		if len(events) == 0 {
			fmt.Println(ui.Warning_("No events found"))
			return nil
		}

		fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("📅 Found %d events", len(events))))
		fmt.Println()

		for _, event := range events {
			printEventSummary(event)
		}

		return nil
	},
}

var calShowCmd = &cobra.Command{
	Use:   "show [event-id]",
	Short: "Show event details",
	Long:  `Display full details of a specific event.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		eventID := args[0]

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		event, err := client.GetEvent(ctx, "", eventID)
		if err != nil {
			return err
		}

		printEventFull(event)
		return nil
	},
}

var calCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new event",
	Long: `Create a new calendar event.

Examples:
  gooctl calendar create --title "Meeting" --start "2024-01-15T10:00:00" --end "2024-01-15T11:00:00"
  gooctl calendar create --title "Birthday" --date "2024-01-15" --all-day
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		location, _ := cmd.Flags().GetString("location")
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		dateStr, _ := cmd.Flags().GetString("date")
		allDay, _ := cmd.Flags().GetBool("all-day")
		attendeesStr, _ := cmd.Flags().GetString("attendees")

		if title == "" {
			return fmt.Errorf("--title is required")
		}

		event := &calpkg.Event{
			Summary:     title,
			Description: description,
			Location:    location,
			AllDay:      allDay,
		}

		// Parse attendees
		if attendeesStr != "" {
			event.Attendees = strings.Split(attendeesStr, ",")
		}

		// Parse times
		if allDay || dateStr != "" {
			if dateStr == "" {
				return fmt.Errorf("--date is required for all-day events")
			}
			date, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
			}
			event.Start = date
			event.End = date.AddDate(0, 0, 1)
			event.AllDay = true
		} else {
			if startStr == "" || endStr == "" {
				return fmt.Errorf("--start and --end are required (or use --date --all-day)")
			}
			start, err := time.Parse(time.RFC3339, startStr)
			if err != nil {
				// Try simpler format
				start, err = time.ParseInLocation("2006-01-02T15:04:05", startStr, time.Local)
				if err != nil {
					return fmt.Errorf("invalid start time format: %w", err)
				}
			}
			end, err := time.Parse(time.RFC3339, endStr)
			if err != nil {
				end, err = time.ParseInLocation("2006-01-02T15:04:05", endStr, time.Local)
				if err != nil {
					return fmt.Errorf("invalid end time format: %w", err)
				}
			}
			event.Start = start
			event.End = end
		}

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		created, err := client.CreateEvent(ctx, "", event)
		if err != nil {
			return err
		}

		fmt.Println(ui.Success("Event created!"))
		fmt.Println(ui.SubtleStyle.Render("Event ID: " + created.ID))
		if created.HtmlLink != "" {
			fmt.Println(ui.SubtleStyle.Render("Link: " + created.HtmlLink))
		}

		return nil
	},
}

var calDeleteCmd = &cobra.Command{
	Use:   "delete [event-id]",
	Short: "Delete an event",
	Long:  `Delete a calendar event by its ID.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		eventID := args[0]

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		if err := client.DeleteEvent(ctx, "", eventID); err != nil {
			return err
		}

		fmt.Println(ui.Success("Event deleted!"))
		return nil
	},
}

var calUpdateCmd = &cobra.Command{
	Use:   "update [event-id]",
	Short: "Update an existing event",
	Long: `Update an existing calendar event.

Examples:
  gooctl calendar update EVENT_ID --start "2024-01-16T10:00:00" --end "2024-01-16T11:00:00"
  gooctl calendar update EVENT_ID --title "New Title"
  gooctl calendar update EVENT_ID --move-days 1  # Move event forward 1 day
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		eventID := args[0]

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		// Get existing event
		existing, err := client.GetEvent(ctx, "", eventID)
		if err != nil {
			return err
		}

		// Apply updates
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		location, _ := cmd.Flags().GetString("location")
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		moveDays, _ := cmd.Flags().GetInt("move-days")

		if title != "" {
			existing.Summary = title
		}
		if description != "" {
			existing.Description = description
		}
		if location != "" {
			existing.Location = location
		}

		// Handle move-days (shifts both start and end)
		if moveDays != 0 {
			existing.Start = existing.Start.AddDate(0, 0, moveDays)
			existing.End = existing.End.AddDate(0, 0, moveDays)
		}

		// Handle explicit start/end times
		if startStr != "" {
			start, err := time.Parse(time.RFC3339, startStr)
			if err != nil {
				start, err = time.ParseInLocation("2006-01-02T15:04:05", startStr, time.Local)
				if err != nil {
					return fmt.Errorf("invalid start time format: %w", err)
				}
			}
			existing.Start = start
		}
		if endStr != "" {
			end, err := time.Parse(time.RFC3339, endStr)
			if err != nil {
				end, err = time.ParseInLocation("2006-01-02T15:04:05", endStr, time.Local)
				if err != nil {
					return fmt.Errorf("invalid end time format: %w", err)
				}
			}
			existing.End = end
		}

		updated, err := client.UpdateEvent(ctx, "", eventID, existing)
		if err != nil {
			return err
		}

		fmt.Println(ui.Success("Event updated! ✨"))
		fmt.Printf("  %s: %s\n", ui.LabelStyle.Render("Title"), updated.Summary)
		if updated.AllDay {
			fmt.Printf("  %s: %s (all day)\n", ui.LabelStyle.Render("When"), updated.Start.Format("Monday, January 2"))
		} else {
			fmt.Printf("  %s: %s, %s - %s\n", ui.LabelStyle.Render("When"),
				updated.Start.Format("Monday, January 2"),
				updated.Start.Format("3:04 PM"),
				updated.End.Format("3:04 PM"))
		}

		return nil
	},
}

var calListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all calendars",
	Long:  `List all calendars you have access to.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		client, err := getCalendarClient(ctx)
		if err != nil {
			return err
		}

		calendars, err := client.ListCalendars(ctx)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(calendars)
		}

		fmt.Println(ui.TitleStyle.Render("📅 Your Calendars"))
		fmt.Println()

		for _, cal := range calendars {
			primary := ""
			if cal.Primary {
				primary = ui.SuccessStyle.Render(" (primary)")
			}
			fmt.Printf("  %s%s\n", cal.Summary, primary)
			fmt.Printf("  %s\n", ui.SubtleStyle.Render("ID: "+cal.ID))
			fmt.Printf("  %s\n", ui.SubtleStyle.Render("Access: "+cal.AccessRole))
			fmt.Println()
		}

		return nil
	},
}

var (
	calendarFlag string
	jsonOut      bool
)

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	rootCmd.AddCommand(calendarCmd)
	calendarCmd.PersistentFlags().StringVarP(&calendarFlag, "calendar", "c", "primary", `Calendar ID, or "all" to read every calendar in the list (see 'gooctl cal list')`)
	calendarCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output JSON (list, today, week, upcoming, search)")

	calendarCmd.AddCommand(calTodayCmd)
	calTodayCmd.Flags().Int64P("limit", "n", 20, "Maximum number of results")

	calendarCmd.AddCommand(calWeekCmd)
	calWeekCmd.Flags().Int64P("limit", "n", 50, "Maximum number of results")

	calendarCmd.AddCommand(calUpcomingCmd)
	calUpcomingCmd.Flags().Int64P("limit", "n", 10, "Maximum number of results")

	calendarCmd.AddCommand(calSearchCmd)
	calSearchCmd.Flags().Int64P("limit", "n", 10, "Maximum number of results")
	calSearchCmd.Flags().Int("days", 30, "Search range in days (past and future)")

	calendarCmd.AddCommand(calShowCmd)

	calendarCmd.AddCommand(calCreateCmd)
	calCreateCmd.Flags().StringP("title", "t", "", "Event title (required)")
	calCreateCmd.Flags().StringP("description", "d", "", "Event description")
	calCreateCmd.Flags().StringP("location", "l", "", "Event location")
	calCreateCmd.Flags().String("start", "", "Start time (RFC3339, or YYYY-MM-DDTHH:MM:SS in local time; set TZ)")
	calCreateCmd.Flags().String("end", "", "End time (RFC3339, or YYYY-MM-DDTHH:MM:SS in local time; set TZ)")
	calCreateCmd.Flags().String("date", "", "Date for all-day events (YYYY-MM-DD)")
	calCreateCmd.Flags().Bool("all-day", false, "Create an all-day event")
	calCreateCmd.Flags().String("attendees", "", "Comma-separated list of attendee emails")

	calendarCmd.AddCommand(calDeleteCmd)
	calendarCmd.AddCommand(calListCmd)

	calendarCmd.AddCommand(calUpdateCmd)
	calUpdateCmd.Flags().StringP("title", "t", "", "New event title")
	calUpdateCmd.Flags().StringP("description", "d", "", "New event description")
	calUpdateCmd.Flags().StringP("location", "l", "", "New event location")
	calUpdateCmd.Flags().String("start", "", "New start time (RFC3339, or YYYY-MM-DDTHH:MM:SS in local time; set TZ)")
	calUpdateCmd.Flags().String("end", "", "New end time (RFC3339, or YYYY-MM-DDTHH:MM:SS in local time; set TZ)")
	calUpdateCmd.Flags().Int("move-days", 0, "Move event by N days (positive=future, negative=past)")
}

func getCalendarClient(ctx context.Context) (*calpkg.Client, error) {
	config, err := auth.NewConfig()
	if err != nil {
		return nil, err
	}

	httpClient, err := config.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	client, err := calpkg.NewClient(ctx, httpClient)
	if err != nil {
		return nil, err
	}
	client.Calendar = calendarFlag
	return client, nil
}

func printEventSummary(event *calpkg.Event) {
	var timeStr string
	if event.AllDay {
		timeStr = "All day"
	} else {
		timeStr = event.Start.Format("3:04 PM") + " - " + event.End.Format("3:04 PM")
	}

	fmt.Printf("  %s  %s\n",
		ui.SubtleStyle.Render(fmt.Sprintf("%-20s", timeStr)),
		event.Summary,
	)
	if event.Location != "" {
		fmt.Printf("  %s  %s\n",
			ui.SubtleStyle.Render(strings.Repeat(" ", 20)),
			ui.SubtleStyle.Render("📍 "+event.Location),
		)
	}
	if event.Calendar != "" {
		fmt.Printf("  %s\n", ui.SubtleStyle.Render("Calendar: "+event.Calendar+" ("+event.CalendarID+")"))
	}
	fmt.Printf("  %s\n", ui.SubtleStyle.Render("ID: "+event.ID))
}

func printEventFull(event *calpkg.Event) {
	var timeStr string
	if event.AllDay {
		timeStr = event.Start.Format("Monday, January 2, 2006") + " (All day)"
	} else {
		if event.Start.Format("2006-01-02") == event.End.Format("2006-01-02") {
			timeStr = event.Start.Format("Monday, January 2, 2006") + "\n" +
				event.Start.Format("3:04 PM") + " - " + event.End.Format("3:04 PM")
		} else {
			timeStr = event.Start.Format("Mon, Jan 2 3:04 PM") + " - " + event.End.Format("Mon, Jan 2 3:04 PM")
		}
	}

	content := ui.LabelStyle.Render("Title:    ") + event.Summary + "\n" +
		ui.LabelStyle.Render("When:     ") + timeStr

	if event.Location != "" {
		content += "\n" + ui.LabelStyle.Render("Location: ") + event.Location
	}

	if event.Organizer != "" {
		content += "\n" + ui.LabelStyle.Render("Organizer:") + " " + event.Organizer
	}

	if len(event.Attendees) > 0 {
		content += "\n" + ui.LabelStyle.Render("Attendees:") + " " + strings.Join(event.Attendees, ", ")
	}

	content += "\n" + ui.LabelStyle.Render("ID:       ") + event.ID

	if event.HtmlLink != "" {
		content += "\n" + ui.LabelStyle.Render("Link:     ") + event.HtmlLink
	}

	fmt.Println(ui.BoxStyle.Render(content))

	if event.Description != "" {
		fmt.Println()
		fmt.Println(ui.LabelStyle.Render("Description:"))
		fmt.Println(event.Description)
	}
}
