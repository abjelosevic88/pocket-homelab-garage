package server

import (
	"net/http"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// handleReports returns the series the Reports tab draws.
func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := vehicleFrom(ctx).vehicle
	today := store.Today()
	monthsBack := queryInt(r, "months")
	if monthsBack <= 0 {
		monthsBack = 24
	}
	months, err := s.store.CostsByMonth(ctx, v.ID, addMonths(today[:7]+"-01", -(monthsBack-1)), today)
	if err != nil {
		s.fail(w, err, "months")
		return
	}
	years, err := s.store.CostsByYear(ctx, v.ID)
	if err != nil {
		s.fail(w, err, "years")
		return
	}
	stations, _ := s.store.Stations(ctx, v.ID)
	fills, _ := s.store.ListFuel(ctx, v.ID)
	// oldest first for the charts
	type pt struct {
		Date     string  `json:"date"`
		Odometer float64 `json:"odometer"`
		Economy  float64 `json:"economy"`
		PricePer float64 `json:"price_per"`
		Volume   float64 `json:"volume"`
		Cost     int64   `json:"cost_home"`
		Distance float64 `json:"distance"`
	}
	series := make([]pt, 0, len(fills))
	for i := len(fills) - 1; i >= 0; i-- {
		f := fills[i]
		pricePer := 0.0
		if f.Volume > 0 {
			pricePer = float64(f.CostHome) / f.Volume
		}
		series = append(series, pt{Date: f.Date, Odometer: f.Odometer, Economy: f.Economy, PricePer: pricePer, Volume: f.Volume, Cost: f.CostHome, Distance: f.Distance})
	}
	odo, _ := s.store.OdometerHistory(ctx, v.ID)
	type op struct {
		Date    string  `json:"date"`
		Reading float64 `json:"reading"`
	}
	odoSeries := make([]op, 0, len(odo))
	for i := len(odo) - 1; i >= 0; i-- {
		odoSeries = append(odoSeries, op{odo[i].Date, odo[i].Reading})
	}
	// category totals for the records
	cats := map[string]int64{}
	recs, _ := s.store.ListRecords(ctx, v.ID, store.RecordFilter{})
	for _, rc := range recs {
		k := rc.Category
		if k == "" {
			k = rc.Kind
		}
		cats[k] += rc.CostHome
	}
	days, _ := s.store.DawarichDays(ctx, v.ID, addDays(today, -365), today)
	writeJSON(w, http.StatusOK, map[string]any{"months": months, "years": years, "stations": stations, "fuel": series, "odometer": odoSeries, "categories": cats, "dawarich_days": days, "currency": v.Currency})
}
