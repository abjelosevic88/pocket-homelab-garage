package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/money"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// ---- Pocket Money ----
//
// A cost logged here can become an expense in the person's Pocket Money.
// With Pocket Core on the box this needs no setup: Garage asks Core for a
// short-lived link token that acts for that person in Money (scopes
// accounts:read + transactions:write, see Core's docs/links.md); without Core
// a Money URL + personal token in Settings does the same. The transaction is
// booked in the Money of whoever logs the cost, and later edits and deletes
// of the record are carried over as that person.

var moneyScopes = []string{"accounts:read", "transactions:write"}

// moneyLink is how Garage reaches one person's Money.
type moneyLink struct {
	client *money.Client
	via    string // core | token
	public string // what a browser opens
}

var linkReasonText = map[string]string{
	"off_person":      "you switched the Garage → Money link off in Pocket Core (Settings → Apps acting for me)",
	"off_admin":       "the Garage → Money link is switched off in Pocket Core (Links)",
	"target_missing":  "Pocket Money is not registered with Pocket Core",
	"not_offered":     "this Pocket Money version does not accept app links yet",
	"not_declared":    "Pocket Core has not seen this Garage version yet; it re-registers within a minute of starting",
	"not_in_audience": "you do not have access to Pocket Money in Pocket Core",
	"unknown_user":    "your email is not a person in Pocket Core",
}

// moneyFor connects to a person's Money: their own token first, else a
// Pocket Core link.
func (s *Server) moneyFor(ctx context.Context, u *store.User) (*moneyLink, error) {
	if u == nil {
		return nil, errors.New("nobody to book for")
	}
	st := s.settingsFor(ctx, u.ID)
	if st.MoneyToken != "" {
		base := st.MoneyURL
		if base == "" {
			base = s.resolveSibling(ctx, "money")
		}
		if base == "" {
			return nil, errors.New("set the Pocket Money URL in Settings → Integrations")
		}
		return &moneyLink{client: money.New(base, st.MoneyToken, 20*time.Second), via: "token", public: base}, nil
	}
	if !s.core.Configured() {
		return nil, errors.New("Pocket Money is not connected: no Pocket Core on this box and no Money token in Settings")
	}
	tok, err := s.core.ActAs(ctx, u.Email, "money", moneyScopes...)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			if t, ok := linkReasonText[le.Reason]; ok {
				return nil, errors.New(t)
			}
		}
		return nil, err
	}
	pub := tok.PublicURL
	if pub == "" {
		pub = tok.URL
	}
	return &moneyLink{client: money.New(tok.URL, tok.AccessToken, 20*time.Second), via: "core", public: pub}, nil
}

// handleMoneyStatus tells the forms and Settings whether to offer booking,
// where, and what was used last for a vehicle (?vehicle=).
func (s *Server) handleMoneyStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	st := s.settingsFor(ctx, u.ID)
	out := map[string]any{"connected": false, "via": "", "error": "", "enabled": st.MoneyEnabled, "offer": false, "core": s.core.Configured(),
		"accounts": []money.Account{}, "default_account": 0, "url": ""}
	ml, err := s.moneyFor(ctx, u)
	if err != nil {
		out["error"] = err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["via"], out["url"] = ml.via, ml.public
	accts, err := ml.client.Accounts(ctx)
	if err != nil {
		out["error"] = err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	vid, _ := strconv.ParseInt(r.URL.Query().Get("vehicle"), 10, 64)
	out["connected"], out["offer"], out["accounts"] = true, st.MoneyEnabled, accts
	out["default_account"] = pickAccount(st, vid, 0, accts)
	writeJSON(w, http.StatusOK, out)
}

// handleMoneyCategories lists the categories usable on an account (?account=).
func (s *Server) handleMoneyCategories(w http.ResponseWriter, r *http.Request) {
	ml, err := s.moneyFor(r.Context(), userFrom(r.Context()))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	acct, _ := strconv.ParseInt(r.URL.Query().Get("account"), 10, 64)
	cats, err := ml.client.Categories(r.Context(), acct)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

func vehicleAccounts(st store.Settings) map[string]int64 {
	m := map[string]int64{}
	_ = json.Unmarshal([]byte(st.MoneyVehicleAccts), &m)
	return m
}

// pickAccount: the one asked for, else the last used for the vehicle, else
// the person's default, else their first everyday account, else the first.
func pickAccount(st store.Settings, vehicleID, asked int64, accts []money.Account) int64 {
	has := func(id int64) bool {
		for _, a := range accts {
			if a.ID == id {
				return true
			}
		}
		return accts == nil // unknown list: trust the id
	}
	for _, id := range []int64{asked, vehicleAccounts(st)[strconv.FormatInt(vehicleID, 10)], st.MoneyAccount} {
		if id > 0 && has(id) {
			return id
		}
	}
	for _, a := range accts {
		if a.Everyday {
			return a.ID
		}
	}
	if len(accts) > 0 {
		return accts[0].ID
	}
	return 0
}

var (
	fuelCategoryRe = regexp.MustCompile(`(?i)gorivo|fuel|benzin|dizel|diesel|petrol|tank|punjenje|charging`)
	carCategoryRe  = regexp.MustCompile(`(?i)\bauto|\bcar\b|vozil|vehicle|automobil|garage|garaža`)
)

// pickCategory: the person's setting for the kind, else a category whose
// name says fuel / car, else none (Money's rules may still file it).
func pickCategory(st store.Settings, fuel bool, cats []money.Category) *int64 {
	want := st.MoneyCategory
	re := carCategoryRe
	if fuel {
		want, re = st.MoneyCategoryFuel, fuelCategoryRe
	}
	if want > 0 {
		for _, c := range cats {
			if c.ID == want {
				return &c.ID
			}
		}
	}
	for _, try := range []*regexp.Regexp{re, carCategoryRe} {
		for i := range cats {
			if try.MatchString(cats[i].Name) {
				return &cats[i].ID
			}
		}
	}
	return nil
}

// moneyTx describes a record or fill-up as a Money expense.
func (s *Server) moneyTx(v *store.Vehicle, kind string, id int64, date, desc, payee, notes string, cost int64, cur string) money.Tx {
	link := ""
	if s.cfg.BaseURL != "" {
		link = fmt.Sprintf("\n%s/vehicles/%d", strings.TrimRight(s.cfg.BaseURL, "/"), v.ID)
	}
	tags := []string{}
	if v.FireflyTag != "" {
		tags = append(tags, v.FireflyTag)
	}
	return money.Tx{Date: date, Amount: cost, Currency: cur, Description: desc, Payee: payee, Tags: tags,
		Notes: strings.TrimSpace("Pocket Garage · "+kind+"\n"+notes) + link, ExternalID: fmt.Sprintf("%s:%d", map[bool]string{true: "fuel", false: "record"}[kind == "fuel"], id)}
}

func recordMoneyTx(s *Server, v *store.Vehicle, rec *store.Record) money.Tx {
	return s.moneyTx(v, rec.Kind, rec.ID, rec.Date, v.Title()+" "+rec.Description, rec.Vendor, rec.Notes, rec.Cost, rec.Currency)
}

func fuelMoneyTx(s *Server, v *store.Vehicle, f *store.Fuel) money.Tx {
	desc := v.Title() + " fuel"
	if f.Station != "" {
		desc += " · " + f.Station
	}
	return s.moneyTx(v, "fuel", f.ID, f.Date, desc, f.Station, fmt.Sprintf("%.2f l at %.0f %s", f.Volume, f.Odometer, v.OdometerUnit), f.Cost, f.Currency)
}

// moneyChoice is what a form sent: whether to book, and where.
type moneyChoice struct {
	Money         *bool  `json:"money"`
	MoneyAccount  *int64 `json:"money_account"`
	MoneyCategory *int64 `json:"money_category"`
}

// wants decides whether to book: the form's tick, else the person's setting.
func (s *Server) wants(ctx context.Context, u *store.User, c moneyChoice) bool {
	if c.Money != nil {
		return *c.Money
	}
	return u != nil && s.settingsFor(ctx, u.ID).MoneyEnabled
}

// book creates the transaction in the person's Money and remembers the
// account for the vehicle. It returns the new transaction id.
func (s *Server) book(ctx context.Context, u *store.User, v *store.Vehicle, fuel bool, tx money.Tx, c moneyChoice) (int64, error) {
	if tx.Amount <= 0 {
		return 0, errors.New("nothing to book: no cost")
	}
	ml, err := s.moneyFor(ctx, u)
	if err != nil {
		return 0, err
	}
	st := s.settingsFor(ctx, u.ID)
	var asked int64
	if c.MoneyAccount != nil {
		asked = *c.MoneyAccount
	}
	var accts []money.Account
	if asked == 0 {
		if accts, err = ml.client.Accounts(ctx); err != nil {
			return 0, err
		}
	}
	tx.AccountID = pickAccount(st, v.ID, asked, accts)
	if tx.AccountID == 0 {
		return 0, errors.New("you have no account in Pocket Money to book on")
	}
	if c.MoneyCategory != nil && *c.MoneyCategory > 0 {
		id := *c.MoneyCategory
		tx.CategoryID = &id
	} else if cats, err := ml.client.Categories(ctx, tx.AccountID); err == nil {
		tx.CategoryID = pickCategory(st, fuel, cats)
	}
	created, err := ml.client.Create(ctx, tx)
	if err != nil {
		return 0, err
	}
	m := vehicleAccounts(st)
	if m[strconv.FormatInt(v.ID, 10)] != tx.AccountID {
		m[strconv.FormatInt(v.ID, 10)] = tx.AccountID
		b, _ := json.Marshal(m)
		st.MoneyVehicleAccts = string(b)
		_ = s.store.SaveUserSettings(ctx, u.ID, st)
	}
	return created.ID, nil
}

// bookRecord books a record when asked and links it; the error (if any)
// travels back on the response so the form can say what went wrong.
func (s *Server) bookRecord(ctx context.Context, u *store.User, v *store.Vehicle, rec *store.Record, c moneyChoice) {
	if !s.wants(ctx, u, c) || rec.Cost <= 0 || rec.MoneyTxID != nil {
		return
	}
	id, err := s.book(ctx, u, v, false, recordMoneyTx(s, v, rec), c)
	if err != nil {
		rec.MoneyError = err.Error()
		s.log.Warn("pocket money: book record", "record", rec.ID, "err", err)
		return
	}
	_ = s.store.SetRecordMoney(ctx, v.ID, rec.ID, id, u.ID)
	rec.MoneyTxID, rec.MoneyUserID = &id, &u.ID
	s.store.LogActivity(ctx, v.ID, u.ID, "booked in Pocket Money", rec.Description)
}

func (s *Server) bookFuel(ctx context.Context, u *store.User, v *store.Vehicle, f *store.Fuel, c moneyChoice) string {
	if !s.wants(ctx, u, c) || f.Cost <= 0 || f.MoneyTxID != nil {
		return ""
	}
	id, err := s.book(ctx, u, v, true, fuelMoneyTx(s, v, f), c)
	if err != nil {
		s.log.Warn("pocket money: book fuel", "fuel", f.ID, "err", err)
		return err.Error()
	}
	_ = s.store.SetFuelMoney(ctx, v.ID, f.ID, id, u.ID)
	s.store.LogActivity(ctx, v.ID, u.ID, "booked in Pocket Money", "fill-up")
	return ""
}

// bookedBy loads the person whose Money holds a linked transaction.
func (s *Server) bookedBy(ctx context.Context, userID *int64) *store.User {
	if userID == nil {
		return nil
	}
	u, err := s.store.GetUser(ctx, *userID)
	if err != nil {
		return nil
	}
	return u
}

// resyncMoney carries an edit over to the linked transaction: a cost of zero
// deletes it, one deleted in Money unlinks it. Returns a message on failure.
func (s *Server) resyncMoney(ctx context.Context, txID, userID *int64, tx money.Tx, unlink func()) string {
	if txID == nil {
		return ""
	}
	u := s.bookedBy(ctx, userID)
	if u == nil {
		return ""
	}
	ml, err := s.moneyFor(ctx, u)
	if err != nil {
		return "not updated in Pocket Money: " + err.Error()
	}
	if tx.Amount <= 0 {
		if err := ml.client.Delete(ctx, *txID); err != nil {
			return "not removed from Pocket Money: " + err.Error()
		}
		unlink()
		return ""
	}
	if err := ml.client.Update(ctx, *txID, tx); err != nil {
		if errors.Is(err, money.ErrNotFound) {
			unlink()
			return ""
		}
		return "not updated in Pocket Money: " + err.Error()
	}
	return ""
}

// unbookMoney deletes the linked transaction (best effort, logged).
func (s *Server) unbookMoney(ctx context.Context, vehicleID int64, txID, userID *int64) {
	if txID == nil {
		return
	}
	u := s.bookedBy(ctx, userID)
	if u == nil {
		return
	}
	ml, err := s.moneyFor(ctx, u)
	if err == nil {
		err = ml.client.Delete(ctx, *txID)
	}
	if err != nil {
		s.log.Warn("pocket money: delete transaction", "tx", *txID, "err", err)
		s.store.LogActivity(ctx, vehicleID, u.ID, "Pocket Money transaction kept", fmt.Sprintf("#%d: %v", *txID, err))
	}
}

// handleBookRecordMoney books an existing record ("→ Money").
func (s *Server) handleBookRecordMoney(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	rec, err := s.store.GetRecord(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "record")
		return
	}
	if rec.MoneyTxID != nil {
		writeErr(w, http.StatusConflict, "already in Pocket Money")
		return
	}
	var c moneyChoice
	_ = decode(r, &c)
	yes := true
	c.Money = &yes
	s.bookRecord(r.Context(), userFrom(r.Context()), v, rec, c)
	if rec.MoneyError != "" {
		writeErr(w, http.StatusBadGateway, rec.MoneyError)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleBookFuelMoney books an existing fill-up.
func (s *Server) handleBookFuelMoney(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	f, err := s.store.GetFuel(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "fuel")
		return
	}
	if f.MoneyTxID != nil {
		writeErr(w, http.StatusConflict, "already in Pocket Money")
		return
	}
	var c moneyChoice
	_ = decode(r, &c)
	yes := true
	c.Money = &yes
	if msg := s.bookFuel(r.Context(), userFrom(r.Context()), v, f, c); msg != "" {
		writeErr(w, http.StatusBadGateway, msg)
		return
	}
	writeJSON(w, http.StatusOK, s.fuelWithStats(r.Context(), v.ID, f.ID))
}
