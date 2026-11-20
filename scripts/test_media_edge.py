"""Test the checked-in stack and the operator check without AWS access or signed URLs."""

from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
import json
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

import yaml

import check_media_edge as edge


TEMPLATE = Path(__file__).resolve().parents[1] / "infrastructure/media-cloudfront.yaml"
DISABLED = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"  # AWS Managed-CachingDisabled
OPTIMIZED = "658327ea-f89d-4fab-a63d-7e88639e58f6"
PROTECTED = {"audio/voices/*", "voice-private/*", "bible/*"}


class StackLoader(yaml.SafeLoader):
    """Retain the scalar CloudFormation references, rather than discarding their tags."""


StackLoader.add_multi_constructor("!", lambda loader, tag, node: {tag: loader.construct_scalar(node)})


class MediaStackTest(unittest.TestCase):
    def setUp(self):
        self.stack = yaml.load(TEMPLATE.read_text(), Loader=StackLoader)
        self.resources = self.stack["Resources"]
        self.distribution = self.resources["Distribution"]["Properties"]["DistributionConfig"]

    def test_private_paths_precede_catalog_and_override_old_origin_headers(self):
        rules = self.distribution.get("CacheBehaviors", [])
        # First match wins at CloudFront. A general rule installed before any
        # of these three could otherwise shadow it for only *some* objects.
        self.assertCountEqual([b["PathPattern"] for b in rules], PROTECTED)
        for b in rules:
            with self.subTest(path=b["PathPattern"]):
                self.assertEqual(b["CachePolicyId"], DISABLED)
                self.assertEqual(b["ViewerProtocolPolicy"], "https-only")
                policy = self.resources[b["ResponseHeadersPolicyId"]["Ref"]]
                self.assertEqual(policy["Type"], "AWS::CloudFront::ResponseHeadersPolicy")
                headers = policy["Properties"]["ResponseHeadersPolicyConfig"]["CustomHeadersConfig"]["Items"]
                cc, = [h for h in headers if h["Header"].lower() == "cache-control"]
                self.assertEqual(cc["Value"], "private, no-store")
                self.assertIs(cc["Override"], True)
        default = self.distribution["DefaultCacheBehavior"]
        self.assertEqual(default["CachePolicyId"], OPTIMIZED)
        self.assertNotIn("ResponseHeadersPolicyId", default)

    def test_all_behaviors_stay_signed_and_read_only_at_the_private_origin(self):
        origin, = self.distribution["Origins"]
        self.assertEqual(origin["DomainName"], {"GetAtt": "MediaBucket.RegionalDomainName"})
        self.assertFalse(origin.get("OriginPath"))
        self.assertEqual(origin["S3OriginConfig"], {"OriginAccessIdentity": ""})
        self.assertEqual(origin["OriginAccessControlId"], {"Ref": "OriginAccessControl"})
        oac = self.resources["OriginAccessControl"]["Properties"]["OriginAccessControlConfig"]
        self.assertEqual((oac["SigningBehavior"], oac["SigningProtocol"], oac["OriginAccessControlOriginType"]),
                         ("always", "sigv4", "s3"))
        block = self.resources["MediaBucket"]["Properties"]["PublicAccessBlockConfiguration"]
        for name in ("BlockPublicAcls", "IgnorePublicAcls", "BlockPublicPolicy", "RestrictPublicBuckets"):
            self.assertIs(block[name], True)
        policy = self.resources["MediaBucketPolicy"]["Properties"]
        self.assertEqual(policy["Bucket"], {"Ref": "MediaBucket"})
        self.assertEqual([s for s in policy["PolicyDocument"]["Statement"] if s["Effect"] == "Allow"], [{
            "Effect": "Allow", "Principal": {"Service": "cloudfront.amazonaws.com"}, "Action": "s3:GetObject",
            "Resource": {"Sub": "${MediaBucket.Arn}/*"},
            "Condition": {"StringEquals": {"AWS:SourceArn": {
                "Sub": "arn:${AWS::Partition}:cloudfront::${AWS::AccountId}:distribution/${Distribution}"}}},
        }])
        self.assertEqual(self.resources["SigningKeyGroup"]["Properties"]["KeyGroupConfig"]["Items"], [{"Ref": "SigningPublicKey"}])
        for b in [self.distribution["DefaultCacheBehavior"], *self.distribution.get("CacheBehaviors", [])]:
            self.assertEqual(b["TargetOriginId"], origin["Id"])
            self.assertEqual(b["TrustedKeyGroups"], [{"Ref": "SigningKeyGroup"}])
            for field in ("TrustedSigners", "FunctionAssociations", "LambdaFunctionAssociations"):
                self.assertFalse(b.get(field))
            self.assertLessEqual(set(b["AllowedMethods"]), {"GET", "HEAD", "OPTIONS"})
            self.assertTrue({"GET", "HEAD"}.issubset(b["AllowedMethods"]))


def collection(*values):
    return {"Quantity": len(values), "Items": list(values)}


class OperatorCheckTest(unittest.TestCase):
    """CloudFront CLI response shape differs from the CloudFormation property shape."""

    def setUp(self):
        self.cli = ["--distribution-id", "Efixture", "--key-pair-id", "Kfixture", "--origin-host", "bucket.s3.example.test",
                    "--media-base-url", "https://media.example.test:443/"]
        self.default = {"TargetOriginId": "s3", "TrustedKeyGroups": dict(collection("group"), Enabled=True),
                        "AllowedMethods": collection("GET", "HEAD", "OPTIONS"), "ViewerProtocolPolicy": "redirect-to-https",
                        "CachePolicyId": OPTIMIZED}
        private = dict(self.default, CachePolicyId=DISABLED, ResponseHeadersPolicyId="headers", ViewerProtocolPolicy="https-only")
        self.rules = collection(*(dict(private, PathPattern=p) for p in sorted(PROTECTED)))
        self.origin = {"Id": "s3", "DomainName": "bucket.s3.example.test", "S3OriginConfig": {"OriginAccessIdentity": ""},
                       "OriginAccessControlId": "oac"}
        self.dist = {"Id": "Efixture", "Status": "Deployed", "DomainName": "cdn.example.test", "ActiveTrustedKeyGroups": {
            **collection({"KeyGroupId": "group", "KeyPairIds": collection("Kfixture")}), "Enabled": True},
            "DistributionConfig": {"Enabled": True, "Aliases": collection("media.example.test"), "Origins": collection(self.origin),
                                   "DefaultCacheBehavior": self.default, "CacheBehaviors": self.rules}}
        self.oac = {"OriginAccessControlOriginType": "s3", "SigningBehavior": "always", "SigningProtocol": "sigv4"}
        self.header = {"Header": "Cache-Control", "Value": "private, no-store", "Override": True}
        self.responses = {
            ("get-distribution", "Efixture"): {"Distribution": self.dist},
            ("get-origin-access-control", "oac"): {"OriginAccessControl": {"Id": "oac", "OriginAccessControlConfig": self.oac}},
            ("get-response-headers-policy", "headers"): {"ResponseHeadersPolicy": {"Id": "headers", "ResponseHeadersPolicyConfig": {
                "CustomHeadersConfig": collection(self.header)}}},
        }
        self.aws = self.enterContext(patch.object(edge.subprocess, "run", side_effect=self.read_config))

    def read_config(self, args, **kwargs):
        self.assertEqual(args, ["aws", "cloudfront", args[2], "--id", args[4], "--output", "json", "--no-cli-pager"])
        self.assertTrue(kwargs["check"])
        return subprocess.CompletedProcess(args, 0, stdout=json.dumps(self.responses[args[2], args[4]]).encode())

    def run_check(self):
        with redirect_stdout(StringIO()) as out, redirect_stderr(StringIO()) as err:
            code = edge.main(self.cli)
        return code, out.getvalue() + err.getvalue()

    def test_reads_deployed_behavior_oac_and_actual_response_policy(self):
        code, summary = self.run_check()
        self.assertEqual(code, 0, summary)
        self.assertEqual(self.aws.call_count, 3)
        self.assertIn("still required", summary)

    def test_configuration_drift_is_not_verified(self):
        rule = self.rules["Items"][0]
        group = self.dist["ActiveTrustedKeyGroups"]["Items"][0]
        for obj, field, bad in [
            (self.dist, "Status", "InProgress"), (self.dist["DistributionConfig"], "Aliases", collection("other.example.test")),
            (self.origin, "OriginPath", "/catalog"), (self.oac, "SigningBehavior", "no-override"),
            (group, "KeyPairIds", collection("other-key")), (self.default, "TrustedKeyGroups", dict(collection(), Enabled=False)),
            (self.default, "CachePolicyId", DISABLED), (rule, "TrustedKeyGroups", dict(collection("other-group"), Enabled=True)),
            (rule, "CachePolicyId", OPTIMIZED), (rule, "ResponseHeadersPolicyId", ""),
            (rule, "PathPattern", "audio/*"), (self.dist["DistributionConfig"], "CacheBehaviors", collection(
                dict(rule, PathPattern="audio/voices/old/*", CachePolicyId=OPTIMIZED), *self.rules["Items"])),
            (self.header, "Override", False), (self.header, "Value", "public, max-age=31536000, immutable"),
        ]:
            with self.subTest(field=field, value=bad), patch.dict(obj, {field: bad}):
                code, summary = self.run_check()
                self.assertEqual(code, 1, summary)
                self.assertIn("NOT VERIFIED", summary)

    def test_no_aws_details_or_url_echoed_on_failure(self):
        self.aws.side_effect = subprocess.CalledProcessError(1, ["aws"], stderr=b"sensitive-fixture-detail")
        code, summary = self.run_check()
        self.assertEqual(code, 1)
        self.assertNotIn("sensitive-fixture-detail", summary)
        self.aws.reset_mock()
        self.cli[-1] = "https://media.example.test/?sensitive-fixture-detail"
        code, summary = self.run_check()
        self.assertEqual(code, 1)
        self.assertNotIn("sensitive-fixture-detail", summary)
        self.aws.assert_not_called()


if __name__ == "__main__":
    unittest.main()
