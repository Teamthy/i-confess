-- Starter ACL for the dedicated public app schema. Run as the object owner,
-- with psql -X -1 -v ON_ERROR_STOP=1 -v runtime_role=... after --migrate.
-- This covers the boot / account smoke test, NOT all production features.
-- Provision schema USAGE separately. New tables deliberately get no blanket grant.
GRANT SELECT ON public.schema_migrations TO :"runtime_role";

-- Canonical catalog bootstrap and reads.
GRANT SELECT, INSERT ON public.collections, public.categories, public.voices,
    public.confession_variants, public.scripture_references, public.content_versions TO :"runtime_role";
GRANT SELECT, INSERT, UPDATE ON public.confessions, public.audio_assets TO :"runtime_role";

-- Registration and authenticated account summary.
GRANT SELECT, INSERT ON public.users, public.subscriptions, public.refresh_tokens TO :"runtime_role";
GRANT INSERT ON public.session_preferences, public.email_verification_tokens TO :"runtime_role";
GRANT SELECT ON public.admin_users TO :"runtime_role";

-- The worker polls the job queue and the trial sweep examines due rows at startup.
GRANT SELECT, UPDATE ON public.jobs, public.trials TO :"runtime_role";
-- Even without a synthesis provider, the rights sweeper checks for due rows.
-- Processing real grants requires a separately reviewed voice lifecycle ACL.
GRANT SELECT (voice_id, status, expires_at, deleted_at) ON public.voice_rights_grants TO :"runtime_role";
