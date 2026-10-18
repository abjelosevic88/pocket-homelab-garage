package server

import (
	"context"
	"fmt"
	"time"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// seedDemo creates a sample car with two years of history so the app has
// something to show (DEMO_DATA=true). Runs once.
func (s *Server) seedDemo(ctx context.Context) error {
	if c := s.store.Counts(ctx); c["vehicles"] > 0 {
		return nil
	}
	owner, err := s.importOwner(ctx)
	if err != nil {
		return nil // no user yet in local mode; demo waits for setup
	}
	today := time.Now()
	d := func(daysAgo int) string { return today.AddDate(0, 0, -daysAgo).Format("2006-01-02") }
	v := &store.Vehicle{OwnerID: owner.ID, Year: 2016, Make: "Škoda", Model: "Octavia", Trim: "1.6 TDI Combi", Plate: "K40-E-123", Kind: "car", FuelType: "diesel", OdometerUnit: "km", TankCapacity: 50, Currency: "EUR", Color: "#c2410c", PurchaseDate: d(760), PurchasePrice: 1150000, PurchaseOdo: 142000, Status: "active", FireflyTag: "octavia", Dawarich: true}
	if err := s.store.CreateVehicle(ctx, v); err != nil {
		return err
	}
	_ = s.store.CreateOdometer(ctx, &store.OdometerReading{VehicleID: v.ID, Date: d(760), Reading: 142000, Notes: "Bought it", Source: "manual"})
	odo := 142000.0
	for i := 740; i > 0; i -= 16 {
		odo += 520 + float64(i%7)*40
		vol := 34 + float64(i%5)*2.5
		price := 1.42 + float64(i%9)*0.03
		f := &store.Fuel{VehicleID: v.ID, Date: d(i), Odometer: odo, Volume: vol, Cost: int64(vol*price*100 + 0.5), Currency: "EUR", FullTank: i%80 != 0, Station: []string{"INA Vrbanja", "Shell Centar", "OMV Lauš", "Nestro Starčevica"}[(i/16)%4], Grade: "diesel", Source: "manual"}
		f.FXRate, f.CostHome = 1, f.Cost
		_ = s.store.CreateFuel(ctx, f)
	}
	last := odo
	recs := []store.Record{
		{Kind: "service", Category: "oil", Date: d(700), Description: "Oil & filter change, 5W-30", Vendor: "Auto servis Marić", Cost: 14500, Odometer: fp(143900)},
		{Kind: "repair", Category: "brakes", Date: d(610), Description: "Front brake pads and discs", Vendor: "Auto servis Marić", Cost: 28900, Odometer: fp(147200), Parts: []store.Part{{Name: "Brembo pads", Qty: 1, Cost: 6500, PartNo: "P85020"}, {Name: "Discs 288 mm", Qty: 2, Cost: 14000}}},
		{Kind: "expense", Category: "registration", Date: d(560), Description: "Registration", Cost: 32000},
		{Kind: "expense", Category: "insurance", Date: d(560), Description: "Insurance (third party + kasko)", Vendor: "Triglav", Cost: 61000},
		{Kind: "service", Category: "tyres", Date: d(420), Description: "4 summer tyres 205/55 R16 Michelin Primacy 4", Vendor: "Vulkanizer Pepi", Cost: 44000, Odometer: fp(152300), Parts: []store.Part{{Name: "Michelin Primacy 4", Qty: 4, Cost: 40000}}},
		{Kind: "service", Category: "oil", Date: d(360), Description: "Oil & filter change, 5W-30", Vendor: "Auto servis Marić", Cost: 15200, Odometer: fp(154100)},
		{Kind: "upgrade", Category: "accessories", Date: d(300), Description: "Android Auto head unit", Vendor: "online", Cost: 24900},
		{Kind: "repair", Category: "battery", Date: d(210), Description: "New battery 72 Ah", Vendor: "Auto dijelovi Jović", Cost: 13900, Odometer: fp(158600)},
		{Kind: "expense", Category: "registration", Date: d(195), Description: "Registration", Cost: 33500},
		{Kind: "expense", Category: "insurance", Date: d(195), Description: "Insurance", Vendor: "Triglav", Cost: 63000},
		{Kind: "service", Category: "timing belt", Date: d(120), Description: "Timing belt + water pump", Vendor: "Auto servis Marić", Cost: 52000, Odometer: fp(161800)},
		{Kind: "expense", Category: "wash", Date: d(20), Description: "Car wash", Cost: 1200},
		{Kind: "expense", Category: "toll", Date: d(9), Description: "Motorway toll Zagreb", Cost: 2400},
	}
	for i := range recs {
		r := recs[i]
		r.VehicleID, r.Currency, r.FXRate, r.CostHome, r.Source = v.ID, "EUR", 1, r.Cost, "manual"
		if r.Parts == nil {
			r.Parts = []store.Part{}
		}
		_ = s.store.CreateRecord(ctx, &r)
	}
	due := fp(last + 1200)
	rems := []store.Reminder{
		{Title: "Oil & filter change", Kind: "service", Category: "oil", DueDate: d(-25), DueOdometer: due, IntervalDays: 365, IntervalKm: 15000},
		{Title: "Registration", Kind: "expense", Category: "registration", DueDate: d(-170), IntervalDays: 365},
		{Title: "Winter tyres on", Kind: "service", Category: "tyres", DueDate: d(-6), IntervalDays: 365},
		{Title: "Brake fluid", Kind: "service", Category: "brakes", DueDate: d(3), IntervalDays: 730},
		{Title: "Cabin filter", Kind: "service", Category: "filters", DueOdometer: fp(last - 300), IntervalKm: 20000},
	}
	for i := range rems {
		r := rems[i]
		r.VehicleID = v.ID
		_ = s.store.CreateReminder(ctx, &r)
	}
	plans := []store.Plan{
		{Title: "Replace rear wiper", Kind: "repair", Priority: "low", Stage: "backlog", Estimate: 1500},
		{Title: "Fix rattling heat shield", Kind: "repair", Category: "exhaust", Priority: "normal", Stage: "planned", Estimate: 4000},
		{Title: "Dashcam install", Kind: "upgrade", Category: "accessories", Priority: "normal", Stage: "in_progress", Estimate: 9000},
		{Title: "Headlight restoration", Kind: "upgrade", Priority: "low", Stage: "testing", Estimate: 2500},
	}
	for i := range plans {
		p := plans[i]
		p.VehicleID = v.ID
		_ = s.store.CreatePlan(ctx, &p)
	}
	_ = s.store.CreateNote(ctx, &store.Note{VehicleID: v.ID, Title: "Tyre pressures", Body: "Front 2.3 bar, rear 2.1 bar (2.5 / 2.7 loaded). Wheel bolts 120 Nm.", Pinned: true})
	_ = s.store.CreateNote(ctx, &store.Note{VehicleID: v.ID, Title: "Radio code", Body: "1 2 3 4 — it is in the service book too."})
	for i := 60; i >= 0; i-- {
		km := 18.0 + float64((i*7)%40)
		if i%7 == 0 {
			km = 0
		}
		_ = s.store.PutDawarichDay(ctx, v.ID, d(i), km, km+2)
	}
	s.store.LogActivity(ctx, v.ID, owner.ID, "demo data seeded", fmt.Sprintf("%d fill-ups", 740/16))
	m := &store.Vehicle{OwnerID: owner.ID, Year: 2021, Make: "Yamaha", Model: "MT-07", Plate: "K12-M-77", Kind: "motorcycle", FuelType: "petrol", OdometerUnit: "km", TankCapacity: 14, Currency: "EUR", Color: "#0284c7", PurchaseDate: d(400), PurchasePrice: 620000, PurchaseOdo: 8000, Status: "active"}
	if err := s.store.CreateVehicle(ctx, m); err == nil {
		_ = s.store.CreateOdometer(ctx, &store.OdometerReading{VehicleID: m.ID, Date: d(400), Reading: 8000, Source: "manual"})
		o := 8000.0
		for i := 380; i > 100; i -= 20 {
			o += 210
			f := &store.Fuel{VehicleID: m.ID, Date: d(i), Odometer: o, Volume: 10.5, Cost: 1680, Currency: "EUR", FullTank: true, Station: "Shell Centar", Grade: "95", Source: "manual", FXRate: 1, CostHome: 1680}
			_ = s.store.CreateFuel(ctx, f)
		}
		r := &store.Record{VehicleID: m.ID, Kind: "service", Category: "oil", Date: d(200), Description: "10,000 km service", Cost: 18000, Currency: "EUR", FXRate: 1, CostHome: 18000, Odometer: fp(10100), Source: "manual", Parts: []store.Part{}}
		_ = s.store.CreateRecord(ctx, r)
		_ = s.store.CreateReminder(ctx, &store.Reminder{VehicleID: m.ID, Title: "Chain lube & tension", Kind: "service", Category: "other", DueOdometer: fp(o + 150), IntervalKm: 800})
	}
	return nil
}

func fp(f float64) *float64 { return &f }
