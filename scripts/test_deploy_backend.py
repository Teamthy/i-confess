"""Exercise the actual release driver and rendered manifests, without cluster credentials."""

import json
import subprocess
import unittest
from unittest.mock import patch

import yaml

import deploy_backend as release


DIGEST = "sha256:" + "a" * 64
IMAGE = release.IMAGE + "@" + DIGEST
JOB = "job.batch/iconfess-migrate-test"


def condition(name):
    return {"status": {"conditions": [{"type": name, "status": "True"}]}}


class ReleaseTest(unittest.TestCase):
    def setUp(self):
        self.deployment = None
        self.pods = []
        self.jobs = []
        self.status = condition("Complete")
        self.live_image = IMAGE
        self.calls = []
        self.job_manifest = None
        self.backend_manifest = None
        self.namespace = "iconfess-staging"
        self.enterContext(patch.object(release.subprocess, "run", side_effect=self.kubectl))
        self.enterContext(patch.object(release.time, "sleep"))

    def kubectl(self, cmd, *, input, text, check, stdout):
        self.assertEqual(cmd[:3], ["kubectl", "-n", self.namespace])
        self.assertTrue(check)
        args = cmd[3:]
        self.calls.append(args)
        output = ""
        if args[:2] == ["get", release.BACKEND]:
            if "--ignore-not-found" in args:
                output = json.dumps(self.deployment) if self.deployment else ""
            else:
                output = self.live_image
        elif args[:2] == ["get", "pods"]:
            output = json.dumps({"items": self.pods})
        elif args[:2] == ["get", "jobs"]:
            output = json.dumps({"items": self.jobs})
        elif args[:2] == ["get", JOB]:
            output = json.dumps(self.status)
        elif args[0] == "create":
            self.job_manifest = yaml.safe_load(input)
            output = JOB
        elif args == ["apply", "-f", "-"]:
            self.backend_manifest = list(yaml.safe_load_all(input))
        return subprocess.CompletedProcess(cmd, 0, stdout=output)

    def test_order_digest_and_secret_isolation(self):
        self.namespace = "iconfess-prod"
        release.deploy("production", DIGEST)
        actions = [a[0] for a in self.calls]
        self.assertLess(self.calls.index(["apply", "-f", str(release.K8S / "migration-policy.yaml")]), actions.index("create"))
        status_check = next(i for i, a in enumerate(self.calls) if a[:2] == ["get", JOB])
        self.assertLess(actions.index("create"), status_check)
        self.assertLess(status_check, self.calls.index(["apply", "-f", "-"]))
        self.assertGreater(actions.index("rollout"), self.calls.index(["apply", "-f", "-"]))

        job = self.job_manifest["spec"]["template"]
        self.assertIn("generateName", self.job_manifest["metadata"])
        self.assertFalse(job["spec"]["automountServiceAccountToken"])
        migrate, = job["spec"]["containers"]
        self.assertEqual(migrate["image"], IMAGE)
        self.assertEqual(migrate["args"], ["--migrate"])
        self.assertNotIn("envFrom", migrate)
        self.assertEqual(migrate["env"], [{"name": "MIGRATION_DATABASE_URL", "valueFrom": {
            "secretKeyRef": {"name": "iconfess-migration-db", "key": "database-url"}}}])

        resources = {doc["kind"]: doc for doc in self.backend_manifest}
        self.assertNotIn("Secret", resources)  # releases never create/replace real credentials
        self.assertNotIn("Namespace", resources)
        self.assertNotIn("iconfess-migration-db", json.dumps(resources))
        for doc in [*self.backend_manifest, self.job_manifest, yaml.safe_load((release.K8S / "migration-policy.yaml").read_text())]:
            self.assertNotIn("namespace", doc["metadata"])  # kubectl -n selects the environment
        backend, = resources["Deployment"]["spec"]["template"]["spec"]["containers"]
        self.assertEqual(backend["image"], IMAGE)
        env = {e["name"]: e for e in backend["env"]}
        self.assertEqual(env["DB_SCHEMA_MODE"]["value"], "verify")
        self.assertEqual(env["DATABASE_URL"]["valueFrom"]["secretKeyRef"], {"name": "iconfess-runtime-db", "key": "database-url"})
        # A migration is not a backend replica and must never receive service traffic.
        self.assertNotEqual(job["metadata"]["labels"]["app"], resources["Service"]["spec"]["selector"]["app"])

    def test_failed_job_never_applies_backend(self):
        self.status = condition("Failed")
        with self.assertRaisesRegex(RuntimeError, "failed"):
            release.deploy("staging", DIGEST)
        self.assertIsNotNone(self.job_manifest)
        self.assertIsNone(self.backend_manifest)
        self.assertNotIn("rollout", [a[0] for a in self.calls])

    def test_pending_job_times_out_without_rollout(self):
        self.status = {"status": {"active": 1}}
        with patch.object(release.time, "monotonic", side_effect=[0, 1, 700]), self.assertRaisesRegex(RuntimeError, "timeout"):
            release.deploy("staging", DIGEST)
        self.assertIsNone(self.backend_manifest)

    def test_migrate_only_stops_for_operator_grants(self):
        release.deploy("staging", DIGEST, migrate_only=True)
        self.assertIsNotNone(self.job_manifest)
        self.assertIsNone(self.backend_manifest)
        self.assertNotIn("rollout", [a[0] for a in self.calls])

    def test_unknown_result_from_cancelled_release_blocks_retry(self):
        self.jobs = [condition("Failed"), {}]  # a Job may not have a status yet
        with self.assertRaisesRegex(RuntimeError, "not terminal"):
            release.deploy("staging", DIGEST)
        self.assertNotIn("create", [a[0] for a in self.calls])

    def test_legacy_template_or_remaining_pod_blocks_ddl(self):
        pod = {"spec": {"containers": [{"name": "backend"}]}}
        self.deployment = {"spec": {"replicas": 1, "template": pod}}
        with self.assertRaisesRegex(RuntimeError, "legacy backend"):
            release.deploy("staging", DIGEST)
        self.deployment["spec"]["replicas"] = 0
        self.pods = [pod]
        with self.assertRaisesRegex(RuntimeError, "legacy backend pod"):
            release.deploy("staging", DIGEST)
        self.assertNotIn("create", [a[0] for a in self.calls])

    def test_ambiguous_mode_is_not_a_cutover(self):
        self.pods = [{"spec": {"containers": [{"name": "backend", "env": [
            {"name": "DB_SCHEMA_MODE", "value": "verify"}, {"name": "DB_SCHEMA_MODE", "value": "migrate"}
        ]}]}}]
        with self.assertRaisesRegex(RuntimeError, "legacy backend pod"):
            release.deploy("staging", DIGEST)
        self.assertIsNone(self.job_manifest)

    def test_subsequent_verify_mode_release(self):
        pod = {"spec": {"containers": [{"name": "backend", "env": [{"name": "DB_SCHEMA_MODE", "value": "verify"}]}]}}
        self.deployment = {"spec": {"replicas": 3, "template": pod}}
        self.pods = [pod]
        self.jobs = [condition("Complete"), condition("Failed")]
        release.deploy("staging", DIGEST)
        self.assertIsNotNone(self.backend_manifest)

    def test_mutable_image_ref_rejected_before_cluster_access(self):
        with self.assertRaisesRegex(ValueError, "digest"):
            release.deploy("staging", "latest")
        self.assertFalse(self.calls)

    def test_different_rolled_out_digest_is_an_error(self):
        self.live_image = release.IMAGE + "@sha256:" + "b" * 64
        with self.assertRaisesRegex(RuntimeError, "approved digest"):
            release.deploy("staging", DIGEST)


if __name__ == "__main__":
    unittest.main()


class BibleLoadTest(ReleaseTest):
    def test_load_bible_runs_the_hardened_job_and_never_touches_the_backend(self):
        release.load_bible("staging", DIGEST)
        job = self.job_manifest
        self.assertEqual(job["metadata"]["labels"]["app"], "iconfess-bible-load")
        pod = job["spec"]["template"]["spec"]
        container = pod["containers"][0]
        self.assertEqual(container["image"], IMAGE)
        self.assertFalse(pod["automountServiceAccountToken"])
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        # The DSN comes from the migration Secret, never from an argument.
        env = {e["name"]: e for e in container["env"]}
        self.assertEqual(env["DATABASE_URL"]["valueFrom"]["secretKeyRef"]["name"], "iconfess-migration-db")
        self.assertNotIn("postgres", " ".join(container["args"]))
        self.assertIsNone(self.backend_manifest)
        self.assertNotIn("rollout", [a[0] for a in self.calls])

    def test_failed_load_is_reported(self):
        self.status = condition("Failed")
        with self.assertRaisesRegex(RuntimeError, "failed"):
            release.load_bible("staging", DIGEST)

    def test_mutable_image_ref_rejected(self):
        with self.assertRaises(ValueError):
            release.load_bible("staging", "latest")
        self.assertEqual(self.calls, [])

