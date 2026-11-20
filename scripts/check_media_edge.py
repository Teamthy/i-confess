#!/usr/bin/env python3
"""Read-only CloudFront configuration check for infrastructure/media-cloudfront.yaml.

Run with an approved operator profile. Does not fetch media, read a signing key,
change a stack, or prove data-plane behavior. See docs/MEDIA-EDGE-VERIFICATION.md.
"""

import argparse
import json
import re
import subprocess
import sys


PRIVATE_PATHS = {"audio/voices/*", "voice-private/*", "bible/*"}
CACHING_DISABLED = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"
CACHING_OPTIMIZED = "658327ea-f89d-4fab-a63d-7e88639e58f6"
LABEL = r"[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?"
HOST = re.compile(rf"(?:{LABEL}\.)+{LABEL}")


class Unverified(Exception):
    """Message is safe for an operator log; never include an AWS response or URL."""


def require(condition, message):
    if not condition:
        raise Unverified(message)


def items(box):
    values = box.get("Items", [])
    require(isinstance(values, list) and box["Quantity"] == len(values), "Incomplete CloudFront collection.")
    return values


def unused(node, field):
    return not items(node.get(field, {"Quantity": 0}))


def resource(operation, resource_id, field):
    # No shell, no data-plane requests, and no AWS CLI output in release logs.
    require(isinstance(resource_id, str) and re.fullmatch(r"[A-Za-z0-9_-]{1,128}", resource_id),
            "Missing or invalid CloudFront resource ID.")
    try:
        result = subprocess.run(["aws", "cloudfront", operation, "--id", resource_id, "--output", "json", "--no-cli-pager"],
                                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                check=True, timeout=30)
        obj = json.loads(result.stdout)[field]
    except (OSError, subprocess.SubprocessError, ValueError, KeyError, TypeError):
        raise Unverified(f"Could not read {field}. Check AWS CLI v2, the selected profile, ID, and read permission.") from None
    require(obj["Id"] == resource_id, f"Unexpected {field} identity.")
    return obj


def check_behavior(rule, origin_id, group_id, private):
    require(rule["TargetOriginId"] == origin_id, "A behavior targets a different origin.")
    groups = rule["TrustedKeyGroups"]
    require(groups["Enabled"] is True and items(groups) == [group_id], "Every behavior must require the active key group.")
    require(not rule.get("TrustedSigners", {}).get("Enabled"), "Unexpected additional trusted signers.")
    require({"GET", "HEAD"} <= set(items(rule["AllowedMethods"])) <= {"GET", "HEAD", "OPTIONS"},
            "Expected read-only media methods.")
    require(unused(rule, "FunctionAssociations") and unused(rule, "LambdaFunctionAssociations"),
            "Edge functions need a separate routing/header review.")
    if private:
        require(rule["ViewerProtocolPolicy"] == "https-only" and rule.get("CachePolicyId") == CACHING_DISABLED,
                "A private path lacks HTTPS-only delivery or Managed-CachingDisabled.")
    else:
        require(rule["ViewerProtocolPolicy"] in ("https-only", "redirect-to-https") and
                rule.get("CachePolicyId") == CACHING_OPTIMIZED and not rule.get("ResponseHeadersPolicyId"),
                "Unexpected catalog default behavior or unreviewed default response policy.")


def check(distribution_id, media_base_url, key_id, origin_host):
    host = media_base_url.removeprefix("https://").removesuffix("/").removesuffix(":443")
    require(media_base_url.startswith("https://") and len(host) <= 253 and HOST.fullmatch(host),
            "Expected the API's bare HTTPS MEDIA_BASE_URL.")
    require(len(origin_host) <= 253 and HOST.fullmatch(origin_host), "Expected the regional S3 origin hostname.")
    dist = resource("get-distribution", distribution_id, "Distribution")
    cfg = dist["DistributionConfig"]
    require(dist["Status"] == "Deployed" and cfg["Enabled"] is True, "Distribution is not enabled and Deployed.")
    names = [dist["DomainName"], *items(cfg.get("Aliases", {"Quantity": 0}))]
    require(host.lower() in {n.lower() for n in names}, "API media host is not this distribution or an exact configured alias.")
    require(not cfg.get("ContinuousDeploymentPolicyId") and unused(cfg, "OriginGroups") and unused(cfg, "CustomErrorResponses"),
            "Traffic splitting, alternate origins or custom error pages need separate review.")

    origins = items(cfg["Origins"])
    require(len(origins) == 1, "Expected the single stack origin.")
    origin = origins[0]
    require(origin["DomainName"].lower() == origin_host.lower() and not origin.get("OriginPath") and
            origin.get("S3OriginConfig", {}).get("OriginAccessIdentity") == "", "Expected the configured root S3 origin using OAC.")
    oac = resource("get-origin-access-control", origin["OriginAccessControlId"], "OriginAccessControl")["OriginAccessControlConfig"]
    require((oac["OriginAccessControlOriginType"], oac["SigningBehavior"], oac["SigningProtocol"]) == ("s3", "always", "sigv4"),
            "S3 origin access control must always sign with SigV4.")

    active = dist["ActiveTrustedKeyGroups"]
    groups = items(active)
    require(active["Enabled"] is True and len(groups) == 1 and key_id in items(groups[0]["KeyPairIds"]),
            "The API's public key ID is not in the expected active key group.")
    group_id = groups[0]["KeyGroupId"]
    rules = items(cfg["CacheBehaviors"])
    # This is a check of OUR stack, not a wildcard auditor for arbitrary CDNs.
    # Refuse unknown ordered behaviors rather than sampling representative keys
    # and missing a narrower first-match exception.
    require(len(rules) == len(PRIVATE_PATHS) and {b["PathPattern"].removeprefix("/") for b in rules} == PRIVATE_PATHS,
            "Missing or unexpected ordered behavior; cannot establish private-path coverage.")
    check_behavior(cfg["DefaultCacheBehavior"], origin["Id"], group_id, private=False)
    for rule in rules:
        check_behavior(rule, origin["Id"], group_id, private=True)
    policy_ids = {b.get("ResponseHeadersPolicyId") for b in rules}
    require(len(policy_ids) == 1, "Private paths must share the reviewed viewer response policy.")
    policy = resource("get-response-headers-policy", policy_ids.pop(), "ResponseHeadersPolicy")["ResponseHeadersPolicyConfig"]
    cc = [h for h in items(policy["CustomHeadersConfig"]) if h["Header"].lower() == "cache-control"]
    require(len(cc) == 1 and cc[0]["Override"] is True and
            {v.strip().lower() for v in cc[0]["Value"].split(",")} == {"private", "no-store"},
            "Viewer Cache-Control must override legacy object metadata with private, no-store.")
    require(not any(h["Header"].lower() == "cache-control" for h in items(policy.get("RemoveHeadersConfig", {"Quantity": 0}))),
            "Response policy must not remove Cache-Control.")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--distribution-id", required=True, help="DistributionId of the approved media stack")
    parser.add_argument("--media-base-url", required=True, help="API's MEDIA_BASE_URL (origin only; never a signed URL)")
    parser.add_argument("--key-pair-id", required=True, help="API's public CLOUDFRONT_KEY_PAIR_ID, not a key file")
    parser.add_argument("--origin-host", required=True, help="expected S3 bucket RegionalDomainName")
    args = parser.parse_args(argv)
    try:
        check(args.distribution_id, args.media_base_url, args.key_pair_id, args.origin_host)
    except Unverified as exc:
        print(f"NOT VERIFIED: {exc}", file=sys.stderr)
        return 1
    except (KeyError, TypeError, AttributeError, ValueError):
        print("NOT VERIFIED: incomplete or unexpected CloudFront response.", file=sys.stderr)
        return 1
    print("CloudFront configuration matches the private media contract. Staging playback, expiry and direct-origin checks are still required.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
