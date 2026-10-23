package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/dawarich"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// ---- location history: Pocket Movement or Dawarich ----
//
// The kilometres driven per day (the odometer estimate) come from one of two
// places, chosen per person in Settings (location_source):
//
//   - movement: Pocket Movement, reached THROUGH POCKET CORE ONLY (an app
//     link acting as the person). Its journeys are already classified by
//     mode, so the driving distance per day comes straight from its
//     Dawarich-compatible /timeline (movement:read). When only the raw
//     points are allowed (movement:points), the driving km are worked out
//     from the points instead.
//   - dawarich: Dawarich, direct or through Core (services.go), as before.
//
// With nothing chosen, Pocket Movement wins whenever it is reachable for the
// person; a Dawarich key saved earlier (or from the environment) does not
// override it, it stays the alternative. When Movement is not reachable
// (no Core, the link switched off, …) Dawarich is used meanwhile, so the
// estimate never breaks.

const (
	srcMovement = "movement"
	srcDawarich = "dawarich"
)

var (
	movementScopes      = []string{"movement:read"}
	movementPointScopes = []string{"movement:points"}
	// movementUseScopes is what the manifest asks Core for.
	movementUseScopes = []string{"movement:read", "movement:points"}
)

// locationSource is one person's way to the kilometres driven right now.
type locationSource struct {
	kind   string           // movement | dawarich
	client *dawarich.Client // Movement speaks Dawarich's API (the key is a pcx_ link token)
	points bool             // movement: only the raw points are allowed
	how    string           // how it reads in a test message
	// why Pocket Movement is not the one in use ("" when it is, or when the
	// person picked Dawarich)
	movementWhy string
}

// movementFor acts as u in Pocket Movement through Pocket Core: the timeline
// when allowed, else the raw points.
func (s *Server) movementFor(ctx context.Context, u *store.User) (*dawarich.Client, bool, error) {
	if !s.core.Configured() {
		return nil, false, errors.New("needs Pocket Core: there is no Pocket Core on this box")
	}
	if u == nil {
		return nil, false, errors.New("nobody to act for")
	}
	tok, err := s.core.ActAs(ctx, u.Email, "movement", movementScopes...)
	points := false
	if err != nil {
		var ptErr error
		if tok, ptErr = s.core.ActAs(ctx, u.Email, "movement", movementPointScopes...); ptErr != nil {
			return nil, false, movementErr(err)
		}
		points = true
	}
	return dawarich.New(tok.URL, tok.AccessToken, 60*time.Second), points, nil
}

func movementErr(err error) error {
	var le *core.LinkError
	if errors.As(err, &le) {
		if t := linkReason("Pocket Movement", le.Reason); t != "" {
			return errors.New(t)
		}
	}
	return err
}

// locationChoice is the source the person picked ("" counts as Movement).
func locationChoice(st store.Settings) string {
	if st.LocationSource == srcDawarich {
		return srcDawarich
	}
	return srcMovement
}

// locationFor is userID's source of kilometres driven, the way they chose,
// falling back on Dawarich while Pocket Movement is not reachable.
func (s *Server) locationFor(ctx context.Context, userID int64) (*locationSource, error) {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("nobody to act for: %w", err)
	}
	st := s.settingsFor(ctx, userID)
	why := ""
	if locationChoice(st) == srcMovement {
		c, points, err := s.movementFor(ctx, u)
		if err == nil {
			how := "Pocket Movement through Pocket Core"
			if points {
				how += " (raw points)"
			}
			return &locationSource{kind: srcMovement, client: c, points: points, how: how}, nil
		}
		why = err.Error()
	}
	dw, conn, err := s.dawarichFor(ctx, userID)
	if err != nil {
		if why != "" {
			return nil, fmt.Errorf("Pocket Movement: %s; Dawarich: %w", why, err)
		}
		return nil, err
	}
	return &locationSource{kind: srcDawarich, client: dw, how: "Dawarich, " + viaText(conn), movementWhy: why}, nil
}

// movementStatus says, without issuing anything, whether Pocket Movement is
// reachable for u: "core" or "" and why not, and whether only the raw
// points are allowed.
func (s *Server) movementStatus(ctx context.Context, u *store.User) (via, why string, points bool) {
	if u == nil || !s.core.Configured() {
		return "", "there is no Pocket Core on this box", false
	}
	on, reason := s.core.LinkOn(ctx, u.Email, "movement", movementScopes...)
	if on {
		return modeCore, "", false
	}
	if on2, _ := s.core.LinkOn(ctx, u.Email, "movement", movementPointScopes...); on2 {
		return modeCore, "", true
	}
	if t := linkReason("Pocket Movement", reason); t != "" {
		return "", t, false
	}
	return "", reason, false
}

// locationStatus is the source in effect for u without issuing anything:
// movement when picked (or nothing picked) and reachable, else dawarich;
// plus whether that one works and Movement's own status.
func (s *Server) locationStatus(ctx context.Context, u *store.User, st store.Settings) (eff string, works bool, mvVia, mvWhy string, mvPoints bool) {
	mvVia, mvWhy, mvPoints = s.movementStatus(ctx, u)
	if locationChoice(st) == srcMovement && mvVia != "" {
		return srcMovement, true, mvVia, mvWhy, mvPoints
	}
	_, dwVia, _, _ := s.serviceStatus(ctx, u, "dawarich", "Dawarich", st.DawarichMode, st.DawarichURL, st.DawarichAPIKey, false)
	return srcDawarich, dwVia != "", mvVia, mvWhy, mvPoints
}

// locationVia is locationStatus's "does it work" part for userID.
func (s *Server) locationVia(ctx context.Context, userID int64) (string, bool) {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return "", false
	}
	eff, works, _, _, _ := s.locationStatus(ctx, u, s.settingsFor(ctx, userID))
	return eff, works
}

// locationName is how a source reads.
func locationName(kind string) string {
	if kind == srcMovement {
		return "Pocket Movement"
	}
	return "Dawarich"
}

// dayKm is one day's driving, in km.
type dayKm struct {
	date      string
	km, total float64
}

// drivingDays reads the driving km per day in [from, to] from a source.
// Dawarich and Movement refuse ranges longer than 31 days: 28-day windows.
func drivingDays(ctx context.Context, src *locationSource, from, to time.Time) ([]dayKm, error) {
	var out []dayKm
	for cur := from; !cur.After(to); cur = cur.AddDate(0, 0, 28) {
		end := cur.AddDate(0, 0, 28)
		if end.After(to) {
			end = to
		}
		if src.points {
			pts, err := src.client.Points(ctx, cur, end, 0)
			if err != nil {
				return out, err
			}
			out = append(out, pointsDriving(pts, time.Local)...)
			continue
		}
		days, err := src.client.Timeline(ctx, cur, end)
		if err != nil {
			return out, err
		}
		for _, d := range days {
			km := d.Summary.ModeDistances["driving"]
			if km == 0 && src.kind == srcDawarich {
				// older Dawarich without modes: fall back to the day total when it is clearly a drive
				if d.Summary.TotalDistance > 15 {
					km = d.Summary.TotalDistance
				}
			}
			total := d.Summary.TotalDistance
			if d.Summary.DistanceUnit == "mi" {
				km *= 1.609344
				total *= 1.609344
			}
			out = append(out, dayKm{date: d.Date, km: km, total: total})
		}
	}
	return out, nil
}

// pointsDriving works the driving km per day out of raw points: a leg
// counts as driving at 25–200 km/h with at most five minutes between the
// two fixes (walking, cycling, flights and gaps are left out).
func pointsDriving(pts []dawarich.Point, loc *time.Location) []dayKm {
	byDay := map[string]*dayKm{}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		dt := b.TS - a.TS
		if dt <= 0 || dt > 300 {
			continue
		}
		km := haversineKm(a.Lat, a.Lng, b.Lat, b.Lng)
		date := time.Unix(a.TS, 0).In(loc).Format("2006-01-02")
		d := byDay[date]
		if d == nil {
			d = &dayKm{date: date}
			byDay[date] = d
		}
		d.total += km
		if kmh := km / (float64(dt) / 3600); kmh >= 25 && kmh <= 200 {
			d.km += km
		}
	}
	out := make([]dayKm, 0, len(byDay))
	for _, d := range byDay {
		d.km, d.total = math.Round(d.km*100)/100, math.Round(d.total*100)/100
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].date < out[j].date })
	return out
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0088
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(h)))
}

// locationKey remembers which source a vehicle's stored days came from.
func locationKey(vehicleID int64) string { return fmt.Sprintf("location:source:%d", vehicleID) }

// syncLocation stores the driving km per day since the last stored day
// (re-reading the last 3 days, since points keep arriving). When the source
// changed since the last sync, the days since the last odometer reading (at
// least 90) are read again from the new one and replace the old ones, so the
// estimate never mixes the two.
func (s *Server) syncLocation(ctx context.Context, src *locationSource, v *store.Vehicle) (int, error) {
	from := time.Now().AddDate(0, 0, -90)
	prev, _, _ := s.store.CacheGet(ctx, locationKey(v.ID))
	switched := prev != src.kind && !(prev == "" && src.kind == srcDawarich) // days stored before sources existed came from Dawarich
	if last := s.store.LastDawarichDay(ctx, v.ID); last != "" && !switched {
		if t, err := store.ParseDate(last); err == nil {
			from = t.AddDate(0, 0, -3)
		}
	} else if switched {
		if _, date := s.store.LatestOdometer(ctx, v.ID); date != "" {
			if t, err := store.ParseDate(date); err == nil && t.Before(from) {
				from = t
			}
		}
		if limit := time.Now().AddDate(-1, 0, 0); from.Before(limit) {
			from = limit
		}
	}
	if v.PurchaseDate != "" {
		if t, err := store.ParseDate(v.PurchaseDate); err == nil && t.After(from) {
			from = t
		}
	}
	days, err := drivingDays(ctx, src, from, time.Now())
	if err != nil {
		if !switched {
			// keep what was read before the error (the old behaviour)
			s.putDays(ctx, v, days)
		}
		return 0, err
	}
	if switched {
		if err := s.store.DeleteDawarichDaysFrom(ctx, v.ID, from.Format("2006-01-02")); err != nil {
			return 0, err
		}
	}
	n := s.putDays(ctx, v, days)
	_ = s.store.CachePut(ctx, locationKey(v.ID), src.kind)
	return n, nil
}

func (s *Server) putDays(ctx context.Context, v *store.Vehicle, days []dayKm) int {
	n := 0
	for _, d := range days {
		km := d.km
		if v.OdometerUnit == "mi" {
			km /= 1.609344
		}
		if err := s.store.PutDawarichDay(ctx, v.ID, d.date, km, d.total); err == nil {
			n++
		}
	}
	return n
}
