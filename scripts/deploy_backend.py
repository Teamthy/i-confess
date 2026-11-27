"""Release the backend by digest. Requires a prepared namespace; see the DB cutover runbook."""

import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
import time


K8S = Path(__file__).resolve().parents[1] / "server/k8s"
IMAGE = "ghcr.io/teamthy/i-confess"
NAMESPACES = {"staging": "iconfess-staging", "production": "iconfess-prod"}
BACKEND = "deployment/iconfess-backend"


def render(filename, image):
    manifest = (K8S / filename).read_text()
    marker = IMAGE + ":IMAGE_DIGEST_REQUIRED"
    if manifest.count(marker) != 1:
        raise ValueError(f"expected one release image in {filename}")
    return manifest.replace(marker, image)


def verify_only(pod_spec):
    for container in pod_spec["containers"]:
        if container["name"] == "backend":
            modes = [e for e in container.get("env", []) if e["name"] == "DB_SCHEMA_MODE"]
            return modes == [{"name": "DB_SCHEMA_MODE", "value": "verify"}]
    return False


def true_conditions(resource):
    return {c["type"] for c in resource.get("status", {}).get("conditions", []) if c["status"] == "True"}


def check_cutover(kube):
    # Older images migrate on startup without taking today's advisory lock.
    # Check both desired state and actual pods (including terminating pods).
    existing = kube("get", BACKEND, "--ignore-not-found", "-o", "json")
    if existing:
        spec = json.loads(existing)["spec"]
        if spec.get("replicas", 1) != 0 and not verify_only(spec["template"]["spec"]):
            raise RuntimeError("legacy backend can still start; follow the DB cutover runbook first")
    pods = json.loads(kube("get", "pods", "-l", "app=iconfess-backend", "-o", "json"))
    for pod in pods["items"]:
        if pod.get("status", {}).get("phase") not in ("Failed", "Succeeded") and not verify_only(pod["spec"]):
            raise RuntimeError("a legacy backend pod is still present; drain it before migrating")
    # Cancelling Actions does not cancel the in-cluster Job. Never delete or
    # replace an unfinished migration in order to start another release.
    jobs = json.loads(kube("get", "jobs", "-l", "app=iconfess-migrate", "-o", "json"))
    if any(not (true_conditions(job) & {"Failed", "Complete"}) for job in jobs["items"]):
        raise RuntimeError("a prior migration Job is not terminal; inspect it before retrying")


def wait_for_migration(kube, job, deadline_seconds=660):
    # kubectl wait --for=condition=Complete also waits to timeout on a Failed
    # Job. Report a terminal failure immediately, without dumping secret-bearing logs.
    deadline = time.monotonic() + deadline_seconds  # Job activeDeadlineSeconds plus controller grace
    while time.monotonic() < deadline:
        conditions = true_conditions(json.loads(kube("get", job, "--request-timeout=15s", "-o", "json")))
        if conditions & {"Failed", "FailureTarget"}:
            raise RuntimeError(f"{job} failed; backend not applied. Inspect the Job in the target namespace.")
        if "Complete" in conditions:
            return
        time.sleep(3)
    raise RuntimeError(f"{job} did not complete before timeout; backend not applied")


def deploy(environment, digest, *, migrate_only=False):
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        raise ValueError("a full lowercase sha256 image digest is required")
    namespace = NAMESPACES[environment]
    image = f"{IMAGE}@{digest}"
    migration = render("migration-job.yaml", image)
    backend = render("deployment-prod.yaml", image)

    def kube(*args, manifest=None):
        # Never fetch Secret contents or pass a DSN to kubectl.
        return subprocess.run(["kubectl", "-n", namespace, *args], input=manifest,
                              text=True, check=True, stdout=subprocess.PIPE).stdout.strip()

    check_cutover(kube)
    if not migrate_only:
        kube("apply", "--dry-run=server", "-f", "-", manifest=backend)
    kube("apply", "-f", str(K8S / "migration-policy.yaml"))
    job = kube("create", "-f", "-", "-o", "name", manifest=migration)
    print(f"{namespace}: waiting for {job} ({image})", flush=True)
    wait_for_migration(kube, job)
    if migrate_only:
        print("Migration complete; backend rollout was not requested.")
        return
    kube("apply", "-f", "-", manifest=backend)
    print(kube("rollout", "status", BACKEND, "--timeout=5m"), flush=True)
    actual = kube("get", BACKEND, "-o", 'jsonpath={.spec.template.spec.containers[?(@.name=="backend")].image}')
    if actual != image:
        raise RuntimeError("the rolled-out backend does not match the approved digest")


def load_bible(environment, digest):
    """Load the registered public-domain Bible text into an environment.

    A one-off per environment (and again when the registry adds an edition),
    not part of every release: it fetches sources from GitHub and the loader
    is idempotent. Runs with the migration role, which owns the Bible tables.
    """
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        raise ValueError("a full lowercase sha256 image digest is required")
    namespace = NAMESPACES[environment]
    image = f"{IMAGE}@{digest}"
    manifest = render("bible-load-job.yaml", image)

    def kube(*args, manifest=None):
        return subprocess.run(["kubectl", "-n", namespace, *args], input=manifest,
                              text=True, check=True, stdout=subprocess.PIPE).stdout.strip()

    job = kube("create", "-f", "-", "-o", "name", manifest=manifest)
    print(f"{namespace}: waiting for {job} ({image})", flush=True)
    wait_for_migration(kube, job, deadline_seconds=1860)
    print("Bible text loaded; the reader can serve it now.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("environment", choices=NAMESPACES)
    parser.add_argument("digest")
    parser.add_argument("--operation", choices=("migrate-and-deploy", "migrate-only", "load-bible"), default="migrate-and-deploy")
    args = parser.parse_args()
    try:
        if args.operation == "load-bible":
            load_bible(args.environment, args.digest)
        else:
            deploy(args.environment, args.digest, migrate_only=args.operation == "migrate-only")
    except (ValueError, RuntimeError, subprocess.CalledProcessError) as exc:
        sys.exit(str(exc))
