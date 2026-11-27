-- 0039: record whether a login session passed a second factor.
--
-- A session is marked mfa_verified when it was created after the account's
-- enrolled second factor (if any) was satisfied. Privileged routes then
-- refuse sessions that are not mfa_verified, so an admin who signed in with
-- only a password - or whose federated assertion bypassed MFA - cannot reach
-- admin surfaces until they complete a second factor.
ALTER TABLE refresh_tokens ADD COLUMN mfa_verified INTEGER NOT NULL DEFAULT 0;
