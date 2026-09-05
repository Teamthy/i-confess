package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
)

// PlanStore is DB-backed catalog; if DB empty it falls back to billing.DefaultPlans.
type PlanStore struct{ db *sql.DB }

func NewPlanStore(db *sql.DB) *PlanStore { return &PlanStore{db: db} }

func (s *PlanStore) List(ctx context.Context) ([]billing.Plan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, interval, trial_days, prices, features FROM subscription_plans WHERE active=1 ORDER BY id`)
	if err != nil {
		return billing.DefaultPlans, nil
	}
	defer rows.Close()
	var out []billing.Plan
	for rows.Next() {
		var p billing.Plan
		var pricesJSON, featuresJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Interval, &p.TrialDays, &pricesJSON, &featuresJSON); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(pricesJSON), &p.Prices)
		_ = json.Unmarshal([]byte(featuresJSON), &p.Features)
		if p.Prices == nil {
			p.Prices = map[string]billing.Amount{}
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return billing.DefaultPlans, nil
	}
	return out, nil
}

func (s *PlanStore) Upsert(ctx context.Context, p billing.Plan) error {
	pricesJSON, _ := json.Marshal(p.Prices)
	featuresJSON, _ := json.Marshal(p.Features)
	_, err := s.db.ExecContext(ctx, `INSERT INTO subscription_plans (id, name, description, interval, trial_days, prices, features, active, created_at, updated_at)
	 VALUES (?,?,?,?,?,?,?,?,?,?)
	 ON CONFLICT(id) DO UPDATE SET name=excluded.name, description=excluded.description, interval=excluded.interval, trial_days=excluded.trial_days, prices=excluded.prices, features=excluded.features, updated_at=excluded.updated_at`,
		p.ID, p.Name, p.Description, p.Interval, p.TrialDays, string(pricesJSON), string(featuresJSON), 1, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	return err
}
