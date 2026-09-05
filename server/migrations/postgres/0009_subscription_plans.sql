-- 0009_subscription_plans.sql — Phase 6 Premium admin-editable pricing
-- DB is source of truth for pricing; mobile never hard-codes amounts.
CREATE TABLE IF NOT EXISTS subscription_plans (
    id          TEXT PRIMARY KEY, -- monthly, annual, promo
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    interval    TEXT NOT NULL CHECK (interval IN ('month','year','week','lifetime')),
    trial_days  INTEGER NOT NULL DEFAULT 7,
    prices      TEXT NOT NULL, -- JSON { "NGN": {"minor":150000,"display":"₦1,500"}, ... }
    features    TEXT NOT NULL DEFAULT '[]', -- JSON array
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_active ON subscription_plans(active) WHERE active=1;

-- Seed from DefaultPlans (billing.DefaultPlans) — illustrative; ops can update via Admin API.
INSERT OR IGNORE INTO subscription_plans (id, name, description, interval, trial_days, prices, features, active, created_at, updated_at) VALUES
 ('monthly', 'Premium Monthly', 'Unlock premium voices, offline downloads and long sessions.', 'month', 7,
  '{"NGN":{"currency":"NGN","minor":150000,"display":"₦1,500"},"USD":{"currency":"USD","minor":499,"display":"$4.99"},"GBP":{"currency":"GBP","minor":399,"display":"£3.99"},"EUR":{"currency":"EUR","minor":499,"display":"€4.99"},"PHP":{"currency":"PHP","minor":29900,"display":"₱299"}}',
  '["premium_voices","offline_downloads","long_sessions"]', 1, datetime('now'), datetime('now')),
 ('annual', 'Premium Annual', 'Premium with annual saving.', 'year', 7,
  '{"NGN":{"currency":"NGN","minor":1200000,"display":"₦12,000"},"USD":{"currency":"USD","minor":3999,"display":"$39.99"},"GBP":{"currency":"GBP","minor":3299,"display":"£32.99"},"EUR":{"currency":"EUR","minor":3999,"display":"€39.99"},"PHP":{"currency":"PHP","minor":199900,"display":"₱1,999"}}',
  '["premium_voices","offline_downloads","long_sessions","annual_saving"]', 1, datetime('now'), datetime('now'));
