# Database cutover and release runbook (G-24)

The Kubernetes release path now runs migrations **before** rolling out the API. This is implemented and locally tested, not a record of a production cutover. Do the rehearsal below on a restored **staging** database before approving production. No workflow creates database roles, sets passwords, creates Secrets, or restores a backup.

## What runs where

| Workload | Command / mode | Credential in the target namespace |
|---|---|---|
| One fresh, bounded Kubernetes Job per release | `iconfess-server --migrate` (no API, seeding or workers) | `iconfess-migration-db`, key `database-url`, as `MIGRATION_DATABASE_URL`. **Direct PostgreSQL or session pooling only.** The lock is session-scoped; transaction pooling is unsafe. |
| Backend Deployment | `iconfess-server`, explicit `DB_SCHEMA_MODE=verify` | `iconfess-runtime-db`, key `database-url`, as `DATABASE_URL`. Never supply the migration URL to the API. |

Both use the **same immutable image digest**. The Job does not import the API's credentials, accept ingress, mount a Kubernetes API token, retry failed migrations, or join the Service. Its egress policy permits cluster DNS and in-namespace PostgreSQL. Each migration commits separately under the existing database advisory lock (30s acquisition timeout). The Job has a 10-minute active deadline; review expensive changes on representative data before scheduling them.

`verify` does not run DDL. Missing/unreadable ledger, missing expected migration or checksum mismatch prevents startup. The server now **also rejects elevated runtime credentials**, including membership of an object-owning role, schema CREATE and table- or column-level ledger writes. Previously verify mode accepted an owner URL. This guard is not an audit of every PostgreSQL capability. Keep a trusted search path, audit ACLs, role membership, delegated functions, default privileges and connection pooling separately. Extra, newer ledger versions are allowed for rolling releases; this does **not** prove schema compatibility.

Non-Kubernetes installations retain the old `DB_SCHEMA_MODE=migrate` default. An unknown mode fails. Outside development/test the API requires `DATABASE_URL` explicitly; `POSTGRES_DSN` is not its connection setting. `--migrate` never falls back to `DATABASE_URL`.

## Prepare each environment

1. **Target and recovery.** Use different databases and protected environments for staging and production. The driver targets `iconfess-staging` and `iconfess-prod` respectively; provision that namespace first. Confirm `AWS_DEPLOY_ROLE_ARN`, `AWS_REGION`, `EKS_CLUSTER`, restricted deployment refs and required reviewers for each GitHub environment. Treat the deployer as privileged: someone who can create a Pod/Job can indirectly access a namespaced Secret. Scope its permissions to the intended release resources. Do not grant the application service account Secret or workload-management permissions.
2. **DB roles.** Provision two distinct, non-superuser logins through the secret manager. The migrator owns the app objects and may create in the dedicated app schema. The runtime login is not the database/schema/object owner, has no administrative role attributes, does not belong to the migrator or another privileged role (even via `SET ROLE`), and needs only schema usage plus reviewed data grants. Point both URLs to the **same database and schema**, using verified TLS/trusted CA and an explicit trusted search path. Ensure a pooler actually connects as the intended role.

   For a **dedicated app database using `public`**, the schema-level starting point below assumes the DBA already created `iconfess_migrator` and `iconfess_runtime` with securely provisioned login credentials. Review effective privileges first; do **not** apply this indiscriminately to a shared database. For an existing installation inventory and transfer ownership of the app tables, sequences and functions during cutover; schema CREATE alone does not grant ALTER on another owner's tables.

   ```sql
   REVOKE CREATE ON SCHEMA public FROM PUBLIC;
   GRANT USAGE, CREATE ON SCHEMA public TO iconfess_migrator;
   GRANT USAGE ON SCHEMA public TO iconfess_runtime;
   ```

3. **Runtime ACL plan.** Keep default privileges restrictive for new objects; in particular never default-grant table writes to the runtime login (that would include a newly created ledger). [The tested starter grants](../server/ops/runtime-smoke-grants.sql) cover ledger SELECT, content bootstrap, the startup polling paths and the account smoke flow, **not the full product**. Maintain a reviewed per-table/operation grant list for enabled features, including sequence USAGE where required, queue processing, billing, moderation and account erasure. No blanket `GRANT ALL`, no runtime grants on unrelated schemas, no ledger writes. Prepare explicit grants for new objects as part of each schema change; use a migration-only release when an operator needs to apply grants before the new code serves traffic.
4. **Cluster configuration.** Provision the two DB Secrets above and `iconfess-runtime-env` (runtime provider configuration, with environment-variable keys such as `JWT_SECRET`, `AUDIO_SIGN_SECRET`, `PUBLIC_BASE_URL`, `EMAIL_PROVIDER=postmark`, `POSTMARK_TOKEN`, `REDIS_PASSWORD` if required, plus the chosen storage and real billing verifier settings). Only runtime credentials belong there. File-based provider keys may be supplied in `iconfess-provider-files`, mounted read-only at `/run/secrets`. Neither DB credentials nor provider keys belong in Git or GitHub workflow inputs. Rotate any historical `CHANGE_ME` values rather than copying them into the new Secrets.
5. **Networking.** Validate DNS, DB TLS/CA and reachability for both pods and Redis/storage/provider access for the API. The checked-in policies assume in-namespace Postgres and Redis. Where infrastructure differs, provision narrowly scoped additional policies, including for external providers or a different DNS resolver. Never disable TLS or allow all egress just to pass a probe. `ENV=production` is intentional in the staging manifest too: use real validation with staging services.
6. **First cutover.** Take a backup and exercise restoration. Confirm the old and new app versions can coexist on the expanded schema. Freeze competing deployers, autoscaling/GitOps reconciliation and any legacy processes or jobs that could migrate. In a maintenance window, quiesce any old migrate-on-startup Deployment and wait for its pods to exit, or first transition to a **known verify-capable** image with a restricted login. Setting an environment variable on an old image that ignores it is not a transition. The workflow checks the Deployment template and nonterminal pods, but cannot find other clusters or off-cluster migrators. Do not switch an old server back on with owner credentials.

## Rehearse and release

- Only after preparing the target, set **`DB_ROLE_SPLIT_READY=true`** in that protected GitHub environment. Leave production unset until the staging rehearsal succeeds. Without this opt-in the workflow stops before cluster access; it does not silently use the legacy path.
- Dispatch **Deploy** with the target, a tested `sha256:` digest and the operation. Use **`migrate-only`** for initial bootstrap or new table grants. It waits for the Job and exits without applying the backend. After migration, the owner can apply the reviewed ACLs. For example, with a securely configured local `psql` service entry for the staging object owner:

  ```bash
  PGSERVICE=iconfess-staging-migrator psql -X -1 -v ON_ERROR_STOP=1 \
    -v runtime_role=iconfess_runtime -f server/ops/runtime-smoke-grants.sql
  ```

  Apply the reviewed **feature** grants too. Do not put URLs/passwords on the command line. Check as the actual runtime login that the ledger can be read, not written, and that it cannot create tables or alter app objects.
- **`migrate-and-deploy`** checks for legacy replicas and unfinished Jobs, dry-runs the backend manifests, installs the Job network policy, creates a uniquely named Job and waits for **that Job's** Complete condition. Only then does it apply the backend, wait for rollout/readiness, and compare the backend image with the requested digest. Re-running the same digest verifies applied migrations instead of reapplying them. Releases are serialized per environment; keep all other deployers out of this workflow's way.
- In staging, verify `/health/ready`, catalog reads, signup and authenticated `/me`, then exercise a representative write and background job for **each enabled feature** with the runtime login. Test a second rollout with old/new replicas coexisting and an intentionally failed migration on a disposable copy. Record the image digest, Job name, ledger versions, ACL review, restore point and observed results (never DSNs) before approving production.

## Failures and rollback

On Job failure, API error or timeout **before backend apply**, no new Deployment is applied. Inspect the named Job/events in its namespace; completed/failed Jobs are retained for 24 hours. A cancelled workflow may leave a Job running. Inspect it and wait for a terminal state before retrying; do not delete a running Job to get around the guard. Earlier migrations in the batch may already be committed. Correct the cause and retry or add a forward migration; **do not edit released SQL or erase ledger rows**.

If migration succeeds but rollout fails, the DB has advanced. Stop and inspect pod events, verify-mode errors, Secret references, ACLs, provider configuration and readiness. The rolling strategy keeps old ready replicas when available; it is not an atomic rollback. Redeploy a previous **verify-capable, schema-compatible** digest with the restricted credential, or ship a forward fix. Use a tested, separately approved restore procedure for data recovery, never automatic down-migration or restoration on deploy failure.

## Evidence boundary

CI exercises `server/cmd/server/migration_integration_test.go` on real PostgreSQL and `scripts/test_deploy_backend.py` with simulated kubectl results; it also validates the Kubernetes resource schemas. The database test uses both non-superuser logins, the starter grants and the real server. It does not certify the full production grant set or cluster. G-24 remains open for the approved staging rehearsal and production cutover.
