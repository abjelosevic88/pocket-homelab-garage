// Package dawarich reads recorded location history: the route actually
// travelled, how far each day, where you stopped. Dawarich is the source of
// truth for "what happened"; the trip is the plan.
package dawarich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to one Dawarich instance.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, key string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and key are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.APIKey != "" }

func (c *Client) get(ctx context.Context, path string, q url.Values, v any) (http.Header, error) {
	if !c.Configured() {
		return nil, errors.New("dawarich is not configured")
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("api_key", c.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	// Dawarich behind APPLICATION_PROTOCOL=https redirects plain http unless told it was https.
	req.Header.Set("X-Forwarded-Proto", "https")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("dawarich rejected the API key (401)")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dawarich HTTP %d", res.StatusCode)
	}
	return res.Header, json.NewDecoder(res.Body).Decode(v)
}

// Health is the connection test.
func (c *Client) Health(ctx context.Context) error {
	var out map[string]any
	_, err := c.get(ctx, "/api/v1/health", nil, &out)
	return err
}

// Point is one recorded position.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	TS  int64   `json:"ts"`
}

// Points returns every recorded point between two instants, oldest first.
func (c *Client) Points(ctx context.Context, from, to time.Time, max int) ([]Point, error) {
	var out []Point
	for page := 1; page < 500; page++ {
		q := url.Values{"start_at": {from.UTC().Format(time.RFC3339)}, "end_at": {to.UTC().Format(time.RFC3339)}, "order": {"asc"}, "slim": {"true"}, "per_page": {"10000"}, "page": {strconv.Itoa(page)}}
		var raw []struct {
			Latitude  json.Number `json:"latitude"`
			Longitude json.Number `json:"longitude"`
			Timestamp int64       `json:"timestamp"`
		}
		h, err := c.get(ctx, "/api/v1/points", q, &raw)
		if err != nil {
			return nil, err
		}
		for _, p := range raw {
			lat, _ := p.Latitude.Float64()
			lng, _ := p.Longitude.Float64()
			out = append(out, Point{Lat: lat, Lng: lng, TS: p.Timestamp})
			if max > 0 && len(out) >= max {
				return out, nil
			}
		}
		total, _ := strconv.Atoi(h.Get("X-Total-Pages"))
		if page >= total || len(raw) == 0 {
			break
		}
	}
	return out, nil
}

// Day is one day of the timeline.
type Day struct {
	Date    string `json:"date"`
	Summary struct {
		TotalDistance  float64            `json:"total_distance"`
		DistanceUnit   string             `json:"distance_unit"`
		PlacesVisited  int                `json:"places_visited"`
		MovingMinutes  int                `json:"time_moving_minutes"`
		StationaryMin  int                `json:"time_stationary_minutes"`
		ModeDistances  map[string]float64 `json:"mode_distances"`
		SuggestedCount int                `json:"suggested_count"`
		ConfirmedCount int                `json:"confirmed_count"`
	} `json:"summary"`
	Entries []Entry `json:"entries"`
}

// Entry is a journey or a visit inside a day.
type Entry struct {
	Type         string   `json:"type"` // journey | visit
	TrackID      *int64   `json:"track_id,omitempty"`
	VisitID      *int64   `json:"visit_id,omitempty"`
	Name         string   `json:"name,omitempty"`
	Status       string   `json:"status,omitempty"`
	StartedAt    string   `json:"started_at"`
	EndedAt      string   `json:"ended_at"`
	Duration     int      `json:"duration"`
	Distance     float64  `json:"distance"`
	DominantMode string   `json:"dominant_mode,omitempty"`
	AvgSpeed     float64  `json:"avg_speed,omitempty"`
	Elevation    float64  `json:"elevation_gain,omitempty"`
	Lat          *float64 `json:"latitude,omitempty"`
	Lng          *float64 `json:"longitude,omitempty"`
	PointCount   int      `json:"point_count,omitempty"`
}

// Timeline returns the day-by-day summary for a date range.
func (c *Client) Timeline(ctx context.Context, from, to time.Time) ([]Day, error) {
	var out struct {
		Days []Day `json:"days"`
	}
	q := url.Values{"start_at": {from.UTC().Format(time.RFC3339)}, "end_at": {to.UTC().Format(time.RFC3339)}}
	if _, err := c.get(ctx, "/api/v1/timeline", q, &out); err != nil {
		return nil, err
	}
	return out.Days, nil
}

// Visit is a stop Dawarich detected.
type Visit struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	StartedAt string   `json:"started_at"`
	EndedAt   string   `json:"ended_at"`
	Duration  int      `json:"duration"` // minutes
	Status    string   `json:"status"`
	Lat       *float64 `json:"lat"`
	Lng       *float64 `json:"lng"`
}

// Visits lists detected stops in a range.
func (c *Client) Visits(ctx context.Context, from, to time.Time) ([]Visit, error) {
	var raw []struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at"`
		Duration  int    `json:"duration"`
		Status    string `json:"status"`
		Place     struct {
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		} `json:"place"`
	}
	q := url.Values{"start_at": {from.UTC().Format(time.RFC3339)}, "end_at": {to.UTC().Format(time.RFC3339)}}
	if _, err := c.get(ctx, "/api/v1/visits", q, &raw); err != nil {
		return nil, err
	}
	out := make([]Visit, 0, len(raw))
	for _, v := range raw {
		out = append(out, Visit{ID: v.ID, Name: v.Name, StartedAt: v.StartedAt, EndedAt: v.EndedAt, Duration: v.Duration, Status: v.Status, Lat: v.Place.Latitude, Lng: v.Place.Longitude})
	}
	return out, nil
}
