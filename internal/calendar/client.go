package calendar

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// Client wraps the Calendar API service
type Client struct {
	service *calendar.Service
	// Calendar is the calendar ID used when a call doesn't name one: "primary" by
	// default, or "all" to read across every calendar in the account's list.
	Calendar string
}

// AllCalendars, as a calendar ID, reads across every calendar in the account's list.
const AllCalendars = "all"

func (c *Client) cal(id string) string {
	if id != "" {
		return id
	}
	if c.Calendar != "" {
		return c.Calendar
	}
	return "primary"
}

func (c *Client) single(id string) (string, error) {
	id = c.cal(id)
	if id == AllCalendars {
		return "", fmt.Errorf("this command needs one calendar: pass --calendar <id> (see 'gooctl cal list')")
	}
	return id, nil
}

// acrossCalendars runs fetch for every calendar in the list, tags each event with
// its calendar, and merges the results by start time.
func (c *Client) acrossCalendars(ctx context.Context, maxResults int64, fetch func(calID string) ([]*Event, error)) ([]*Event, error) {
	cals, err := c.ListCalendars(ctx)
	if err != nil {
		return nil, err
	}
	var all []*Event
	for _, cal := range cals {
		events, err := fetch(cal.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cal.Summary, err)
		}
		for _, e := range events {
			e.Calendar = cal.Summary
			e.CalendarID = cal.ID
		}
		all = append(all, events...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Start.Before(all[j].Start) })
	if maxResults > 0 && int64(len(all)) > maxResults {
		all = all[:maxResults]
	}
	return all, nil
}

// Event represents a calendar event
type Event struct {
	ID          string    `json:"id"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	Location    string    `json:"location,omitempty"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Attendees   []string  `json:"attendees,omitempty"`
	HtmlLink    string    `json:"htmlLink,omitempty"`
	Status      string    `json:"status,omitempty"`
	Creator     string    `json:"creator,omitempty"`
	Organizer   string    `json:"organizer,omitempty"`
	Calendar    string    `json:"calendar,omitempty"`
	CalendarID  string    `json:"calendarId,omitempty"`
}

// CalendarInfo represents a calendar
type CalendarInfo struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	Primary     bool   `json:"primary"`
	AccessRole  string `json:"accessRole"`
}

// NewClient creates a new Calendar client
func NewClient(ctx context.Context, httpClient *http.Client) (*Client, error) {
	service, err := calendar.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Calendar service: %w", err)
	}

	return &Client{
		service: service,
	}, nil
}

// ListCalendars returns all calendars the user has access to
func (c *Client) ListCalendars(ctx context.Context) ([]*CalendarInfo, error) {
	resp, err := c.service.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list calendars: %w", err)
	}

	var calendars []*CalendarInfo
	for _, item := range resp.Items {
		calendars = append(calendars, &CalendarInfo{
			ID:          item.Id,
			Summary:     item.Summary,
			Description: item.Description,
			Primary:     item.Primary,
			AccessRole:  item.AccessRole,
		})
	}

	return calendars, nil
}

// ListEvents lists events within a time range
func (c *Client) ListEvents(ctx context.Context, calendarID string, start, end time.Time, maxResults int64) ([]*Event, error) {
	calendarID = c.cal(calendarID)
	if maxResults <= 0 {
		maxResults = 10
	}
	if calendarID == AllCalendars {
		return c.acrossCalendars(ctx, maxResults, func(id string) ([]*Event, error) {
			return c.ListEvents(ctx, id, start, end, maxResults)
		})
	}

	call := c.service.Events.List(calendarID).
		TimeMin(start.Format(time.RFC3339)).
		TimeMax(end.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		MaxResults(maxResults)

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	var events []*Event
	for _, item := range resp.Items {
		events = append(events, parseEvent(item))
	}

	return events, nil
}

// Search searches for events matching a query
func (c *Client) Search(ctx context.Context, query string, start, end time.Time, maxResults int64) ([]*Event, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	return c.searchIn(ctx, c.cal(""), query, start, end, maxResults)
}

func (c *Client) searchIn(ctx context.Context, calendarID, query string, start, end time.Time, maxResults int64) ([]*Event, error) {
	if calendarID == AllCalendars {
		return c.acrossCalendars(ctx, maxResults, func(id string) ([]*Event, error) {
			return c.searchIn(ctx, id, query, start, end, maxResults)
		})
	}

	call := c.service.Events.List(calendarID).
		Q(query).
		TimeMin(start.Format(time.RFC3339)).
		TimeMax(end.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		MaxResults(maxResults)

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to search events: %w", err)
	}

	var events []*Event
	for _, item := range resp.Items {
		events = append(events, parseEvent(item))
	}

	return events, nil
}

// GetEvent retrieves a single event by ID
func (c *Client) GetEvent(ctx context.Context, calendarID, eventID string) (*Event, error) {
	calendarID, err := c.single(calendarID)
	if err != nil {
		return nil, err
	}

	item, err := c.service.Events.Get(calendarID, eventID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	return parseEvent(item), nil
}

// CreateEvent creates a new calendar event
func (c *Client) CreateEvent(ctx context.Context, calendarID string, event *Event) (*Event, error) {
	calendarID, err := c.single(calendarID)
	if err != nil {
		return nil, err
	}

	gEvent := &calendar.Event{
		Summary:     event.Summary,
		Description: event.Description,
		Location:    event.Location,
	}

	if event.AllDay {
		gEvent.Start = &calendar.EventDateTime{
			Date: event.Start.Format("2006-01-02"),
		}
		gEvent.End = &calendar.EventDateTime{
			Date: event.End.Format("2006-01-02"),
		}
	} else {
		gEvent.Start = &calendar.EventDateTime{
			DateTime: event.Start.Format(time.RFC3339),
		}
		gEvent.End = &calendar.EventDateTime{
			DateTime: event.End.Format(time.RFC3339),
		}
	}

	// Add attendees
	for _, email := range event.Attendees {
		gEvent.Attendees = append(gEvent.Attendees, &calendar.EventAttendee{
			Email: email,
		})
	}

	created, err := c.service.Events.Insert(calendarID, gEvent).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create event: %w", err)
	}

	return parseEvent(created), nil
}

// UpdateEvent updates an existing event
func (c *Client) UpdateEvent(ctx context.Context, calendarID, eventID string, event *Event) (*Event, error) {
	calendarID, err := c.single(calendarID)
	if err != nil {
		return nil, err
	}

	gEvent := &calendar.Event{
		Summary:     event.Summary,
		Description: event.Description,
		Location:    event.Location,
	}

	if event.AllDay {
		gEvent.Start = &calendar.EventDateTime{
			Date: event.Start.Format("2006-01-02"),
		}
		gEvent.End = &calendar.EventDateTime{
			Date: event.End.Format("2006-01-02"),
		}
	} else {
		gEvent.Start = &calendar.EventDateTime{
			DateTime: event.Start.Format(time.RFC3339),
		}
		gEvent.End = &calendar.EventDateTime{
			DateTime: event.End.Format(time.RFC3339),
		}
	}

	updated, err := c.service.Events.Update(calendarID, eventID, gEvent).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to update event: %w", err)
	}

	return parseEvent(updated), nil
}

// DeleteEvent deletes an event
func (c *Client) DeleteEvent(ctx context.Context, calendarID, eventID string) error {
	calendarID, err := c.single(calendarID)
	if err != nil {
		return err
	}

	err = c.service.Events.Delete(calendarID, eventID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}

	return nil
}

// Today returns events for today
func (c *Client) Today(ctx context.Context, maxResults int64) ([]*Event, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)

	return c.ListEvents(ctx, "", start, end, maxResults)
}

// Week returns events for the current week
func (c *Client) Week(ctx context.Context, maxResults int64) ([]*Event, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 7)

	return c.ListEvents(ctx, "", start, end, maxResults)
}

// Upcoming returns upcoming events starting from now
func (c *Client) Upcoming(ctx context.Context, maxResults int64) ([]*Event, error) {
	now := time.Now()
	end := now.AddDate(0, 1, 0) // Next month

	return c.ListEvents(ctx, "", now, end, maxResults)
}

// parseEvent converts a Calendar API event to our Event type
func parseEvent(item *calendar.Event) *Event {
	event := &Event{
		ID:          item.Id,
		Summary:     item.Summary,
		Description: item.Description,
		Location:    item.Location,
		HtmlLink:    item.HtmlLink,
		Status:      item.Status,
	}

	// Parse creator
	if item.Creator != nil {
		event.Creator = item.Creator.Email
	}

	// Parse organizer
	if item.Organizer != nil {
		event.Organizer = item.Organizer.Email
	}

	// Parse attendees
	for _, attendee := range item.Attendees {
		event.Attendees = append(event.Attendees, attendee.Email)
	}

	// Parse start time
	if item.Start != nil {
		if item.Start.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, item.Start.DateTime); err == nil {
				event.Start = t
			}
		} else if item.Start.Date != "" {
			event.AllDay = true
			if t, err := time.Parse("2006-01-02", item.Start.Date); err == nil {
				event.Start = t
			}
		}
	}

	// Parse end time
	if item.End != nil {
		if item.End.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, item.End.DateTime); err == nil {
				event.End = t
			}
		} else if item.End.Date != "" {
			if t, err := time.Parse("2006-01-02", item.End.Date); err == nil {
				event.End = t
			}
		}
	}

	return event
}
