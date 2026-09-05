package billing

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Plan defines a subscription plan with regional pricing.
// Prices are never hard-coded in mobile; they come from GET /v1/subscriptions/plans.
type Plan struct {
	ID          string            `json:"id"`          // monthly, annual, promo
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Interval    string            `json:"interval"`    // month | year
	TrialDays   int               `json:"trial_days"`
	Prices      map[string]Amount `json:"prices"`      // currency -> amount
	Features    []string          `json:"features"`
}

type Amount struct {
	Currency string `json:"currency"` // NGN, USD, GBP, EUR, PHP
	Minor    int64  `json:"minor"`    // minor units (kobo, cents)
	Display  string `json:"display"`  // formatted, e.g. "₦500"
}

// Known currencies — never hard-coded amounts, but currencies are fixed.
var Currencies = []string{"NGN", "USD", "GBP", "EUR", "PHP"}

// DefaultPlans is the catalog seeded to DB; amounts are illustrative and
// overridden by DB/config. Mobile must fetch via API, never embed.
var DefaultPlans = []Plan{
	{
		ID:        "monthly",
		Name:      "Premium Monthly",
		Interval:  "month",
		TrialDays: 7,
		Prices: map[string]Amount{
			"NGN": {Currency: "NGN", Minor: 150000, Display: "₦1,500"},
			"USD": {Currency: "USD", Minor: 499, Display: "$4.99"},
			"GBP": {Currency: "GBP", Minor: 399, Display: "£3.99"},
			"EUR": {Currency: "EUR", Minor: 499, Display: "€4.99"},
			"PHP": {Currency: "PHP", Minor: 29900, Display: "₱299"},
		},
		Features: []string{"premium_voices", "offline_downloads", "long_sessions"},
	},
	{
		ID:        "annual",
		Name:      "Premium Annual",
		Interval:  "year",
		TrialDays: 7,
		Prices: map[string]Amount{
			"NGN": {Currency: "NGN", Minor: 1200000, Display: "₦12,000"},
			"USD": {Currency: "USD", Minor: 3999, Display: "$39.99"},
			"GBP": {Currency: "GBP", Minor: 3299, Display: "£32.99"},
			"EUR": {Currency: "EUR", Minor: 3999, Display: "€39.99"},
			"PHP": {Currency: "PHP", Minor: 199900, Display: "₱1,999"},
		},
		Features: []string{"premium_voices", "offline_downloads", "long_sessions", "annual_saving"},
	},
}

// Validate ensures a plan has all currencies.
func (p Plan) Validate() error {
	for _, cur := range Currencies {
		if _, ok := p.Prices[cur]; !ok {
			return fmt.Errorf("plan %s missing price for %s", p.ID, cur)
		}
	}
	return nil
}

// AmountFor returns the amount for a currency, case-insensitive.
func (p Plan) AmountFor(currency string) (Amount, bool) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	a, ok := p.Prices[currency]
	return a, ok
}

// MarshalJSON ensures prices are not nil.
func (p Plan) MarshalJSON() ([]byte, error) {
	type Alias Plan
	return json.Marshal(struct {
		Alias
	}{Alias: Alias(p)})
}
