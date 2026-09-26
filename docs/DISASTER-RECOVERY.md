# iCONFESS Disaster Recovery (DR) & Business Continuity Plan

**Document Version:** 1.0  
**Effective Date:** 2026-09-20  
**Classification:** Operational Security & Resilience  

---

## 1. Objectives & SLA / SLO Targets

| Objective | Target | Notes |
| :--- | :--- | :--- |
| **Recovery Point Objective (RPO)** | **< 15 minutes** | Achieved via PostgreSQL WAL archiving / PITR + S3 Cross-Region Replication (CRR) |
| **Recovery Time Objective (RTO)** | **< 30 minutes** | Infrastructure as Code (Terraform) + automated container spin-up + restore scripts |
| **Data Integrity Verification** | **100% SHA-256** | All backups validated with cryptographically verifiable checksums |
| **High Availability Target** | **99.95%** | Multi-AZ database cluster, stateless container replicas behind load balancer |

---

## 2. Backup Architecture & Cadence

### 2.1 PostgreSQL Relational Data
- **Continuous WAL Archiving**: Continuous point-in-time recovery (PITR) enabled via WAL segment archiving to cold S3 bucket with 35-day retention.
- **Nightly Full Snapshots**: `pg_dump -F c` snapshots taken nightly at 02:00 UTC using `scripts/backup.sh`.
- **Encrypted Storage**: Backups encrypted in transit (TLS 1.3) and at rest (AWS KMS Customer Managed Keys / AES-256-GCM).

### 2.2 S3 Media Storage (Audio, Avatars, Assets)
- **Object Versioning**: Enabled across all production buckets (`iconfess-media-prod`).
- **Cross-Region Replication (CRR)**: Asynchronously replicates canonical audio and user assets to a secondary geographical region (e.g., `eu-west-1` to `eu-central-1`).
- **Lifecycle Policies**: Transition older asset versions to Glacier Instant Retrieval after 90 days.

---

## 3. Disaster Scenarios & Recovery Runbooks

### Scenario A: Catastrophic Database Corruption / Accidental Drop
1. **Declare Incident & Freeze Ingress**:
   - Put CDN / Load Balancer into maintenance mode (HTTP 503).
2. **Provision Target DB Instance**:
   - Ensure clean PostgreSQL 17 instance is available.
3. **Execute Restore**:
   ```bash
   ./scripts/restore.sh /var/backups/iconfess/iconfess_db_latest.dump "postgres://user:pass@host:5432/iconfess?sslmode=verify-full"
   ```
4. **Replay WAL Logs (PITR)**:
   - Replay logs up to the transaction prior to corruption timestamp.
5. **Run Sanity Probes**:
   - Run `/health/ready` probe and verify count of canonical confessions, voices, and user records.
6. **Reopen Traffic**:
   - Disable maintenance mode on CDN.

---

### Scenario B: Cloud Provider Regional Outage
1. **Activate Secondary Region**:
   - Deploy container fleet via Kubernetes / Terraform in failover region.
2. **Promote Read Replica**:
   - Promote cross-region PostgreSQL replica to primary.
3. **Switch DNS / CDN Routing**:
   - Update Route53 / Cloudflare DNS records to point to secondary region ALB.
4. **Verify S3 Replication**:
   - Point application `S3_BUCKET` configuration to secondary regional replica bucket.

---

## 4. Disaster Recovery Testing Cadence

- **Quarterly Automated Tabletop Drills**: Automated restoration of random snapshot to staging environment with data integrity checks.
- **Biannual Full Failover Simulation**: Chaos engineering drill simulating primary region network partition.
- **Rule of Record**: *A backup never restored is not proven recoverable.* Every quarterly drill must log SHA-256 checksums, restore duration, and verification query results to the internal compliance portal.
