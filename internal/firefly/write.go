package firefly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Account is a Firefly asset account.
type Account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// Accounts lists asset accounts (where a car expense is paid from).
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var out []Account
	for page := 1; page < 20; page++ {
		var res struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name         string `json:"name"`
					CurrencyCode string `json:"currency_code"`
					Active       bool   `json:"active"`
				} `json:"attributes"`
			} `json:"data"`
			Meta struct {
				Pagination struct {
					TotalPages int `json:"total_pages"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := c.do(ctx, "/api/v1/accounts", url.Values{"type": {"asset"}, "limit": {"100"}, "page": {strconv.Itoa(page)}}, &res); err != nil {
			return nil, err
		}
		for _, d := range res.Data {
			if d.Attributes.Active {
				out = append(out, Account{ID: d.ID, Name: d.Attributes.Name, Currency: d.Attributes.CurrencyCode})
			}
		}
		if page >= res.Meta.Pagination.TotalPages {
			break
		}
	}
	return out, nil
}

// Categories lists category names.
func (c *Client) Categories(ctx context.Context) ([]string, error) {
	var out []string
	for page := 1; page < 20; page++ {
		var res struct {
			Data []struct {
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
			Meta struct {
				Pagination struct {
					TotalPages int `json:"total_pages"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := c.do(ctx, "/api/v1/categories", url.Values{"limit": {"100"}, "page": {strconv.Itoa(page)}}, &res); err != nil {
			return nil, err
		}
		for _, d := range res.Data {
			out = append(out, d.Attributes.Name)
		}
		if page >= res.Meta.Pagination.TotalPages {
			break
		}
	}
	return out, nil
}

// Withdrawal is what Pocket Garage pushes into Firefly.
type Withdrawal struct {
	Date        string // YYYY-MM-DD
	Amount      string // "12.50" in Currency
	Currency    string
	Description string
	Source      string // asset account name or id
	Destination string // expense account name (created on the fly)
	Category    string
	Tags        []string
	Notes       string
}

// CreateWithdrawal posts one transaction and returns the journal id.
func (c *Client) CreateWithdrawal(ctx context.Context, w Withdrawal) (string, error) {
	if !c.Configured() {
		return "", errors.New("firefly is not configured")
	}
	if w.Source == "" {
		return "", errors.New("a source account is required (Settings → Integrations → Firefly)")
	}
	tx := map[string]any{
		"type": "withdrawal", "date": w.Date, "amount": w.Amount, "description": w.Description,
		"currency_code": w.Currency, "tags": w.Tags, "notes": w.Notes,
	}
	if _, err := strconv.Atoi(w.Source); err == nil {
		tx["source_id"] = w.Source
	} else {
		tx["source_name"] = w.Source
	}
	if w.Destination != "" {
		tx["destination_name"] = w.Destination
	}
	if w.Category != "" {
		tx["category_name"] = w.Category
	}
	body, _ := json.Marshal(map[string]any{"error_if_duplicate_hash": false, "apply_rules": true, "fire_webhooks": true, "transactions": []any{tx}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/transactions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(b, &e)
		return "", fmt.Errorf("firefly HTTP %d: %s", res.StatusCode, strings.TrimSpace(e.Message))
	}
	var out struct {
		Data struct {
			Attributes struct {
				Transactions []struct {
					JournalID string `json:"transaction_journal_id"`
				} `json:"transactions"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Data.Attributes.Transactions) == 0 {
		return "", errors.New("firefly returned no transaction")
	}
	return out.Data.Attributes.Transactions[0].JournalID, nil
}
