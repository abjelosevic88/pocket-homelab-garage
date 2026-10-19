// Package money books car costs in Pocket Money: list where a person can
// book (accounts, categories) and create, change or delete the transaction a
// record or fill-up became. The bearer is either a Pocket Core link token
// (pcx_, acting for the person) or the person's own Pocket Money token.
package money

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotFound is a 404 from Money (the transaction was deleted there).
var ErrNotFound = errors.New("not found in Pocket Money")

// Client is one person's view of their Pocket Money.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if !c.Configured() {
		return errors.New("pocket money is not connected")
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	switch {
	case res.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case res.StatusCode == http.StatusUnauthorized:
		return errors.New("pocket money refused the token (401)")
	case res.StatusCode >= 300:
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = http.StatusText(res.StatusCode)
		}
		return fmt.Errorf("pocket money: %s", e.Error)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Account is a Money account the person can see.
type Account struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Currency string `json:"currency"`
	Everyday bool   `json:"everyday"`
	Archived bool   `json:"archived"`
	Role     string `json:"role"` // owner | editor | viewer
}

// Bookable reports whether the person may book on it.
func (a Account) Bookable() bool { return !a.Archived && a.Role != "viewer" }

// Accounts lists the accounts the person can book on.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var all []Account
	if err := c.do(ctx, http.MethodGet, "/api/v1/accounts", nil, &all); err != nil {
		return nil, err
	}
	out := []Account{}
	for _, a := range all {
		if a.Bookable() {
			out = append(out, a)
		}
	}
	return out, nil
}

// Category is a Money category.
type Category struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Kind     string `json:"kind"`
	Icon     string `json:"icon"`
	Archived bool   `json:"archived"`
}

// Categories lists the expense categories that can be used on an account
// (a shared account books in its owner's categories).
func (c *Client) Categories(ctx context.Context, accountID int64) ([]Category, error) {
	var all []Category
	path := "/api/v1/categories"
	if accountID > 0 {
		path = fmt.Sprintf("/api/v1/accounts/%d/categories", accountID)
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &all); err != nil {
		return nil, err
	}
	out := []Category{}
	for _, ct := range all {
		if !ct.Archived && ct.Kind != "income" {
			out = append(out, ct)
		}
	}
	return out, nil
}

// Tx is what Garage books: always an expense, in the cost's own currency
// (Money converts to the account's).
type Tx struct {
	AccountID   int64    `json:"account_id"`
	Date        string   `json:"date"`
	Amount      int64    `json:"amount"` // minor units of Currency
	Currency    string   `json:"currency"`
	Description string   `json:"description"`
	Payee       string   `json:"payee"`
	CategoryID  *int64   `json:"category_id,omitempty"`
	Tags        []string `json:"tags"`
	Notes       string   `json:"notes"`
	ExternalID  string   `json:"external_id,omitempty"`
}

// Created is the part of Money's answer Garage keeps.
type Created struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Source      string `json:"source"`
}

// Create books a new expense.
func (c *Client) Create(ctx context.Context, t Tx) (*Created, error) {
	body := map[string]any{"kind": "expense", "account_id": t.AccountID, "date": t.Date, "amount": t.Amount, "currency": t.Currency,
		"description": t.Description, "payee": t.Payee, "tags": nonNil(t.Tags), "notes": t.Notes, "source": "garage", "external_id": t.ExternalID}
	if t.CategoryID != nil {
		body["category_id"] = *t.CategoryID
	}
	var out Created
	if err := c.do(ctx, http.MethodPost, "/api/v1/transactions", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes what Garage owns on the transaction: date, amount,
// description and payee. Account, category and tags stay as the person left
// them in Money.
func (c *Client) Update(ctx context.Context, id int64, t Tx) error {
	body := map[string]any{"date": t.Date, "amount": t.Amount, "currency": t.Currency, "description": t.Description, "payee": t.Payee}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/transactions/%d", id), body, nil)
}

// Delete removes the transaction; one already gone counts as deleted.
func (c *Client) Delete(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/transactions/%d", id), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
