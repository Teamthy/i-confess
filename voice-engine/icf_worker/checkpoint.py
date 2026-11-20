"""Content address of an immutable checkpoint directory, shared by trainer and worker."""

from __future__ import annotations

import hashlib
import os
import re
from pathlib import Path


def bundle_sha256(root: Path) -> str:
    """Hash relative names and file digests, independent of the mount point.

    File symlinks (common in model snapshots) contribute the target bytes.
    Do not silently omit directory symlinks, special files or unreadable files.
    Deploy the tree and any targets read-only for the worker's whole lifetime.
    """

    def files(directory: Path):
        for p in sorted(directory.iterdir()):
            if p.is_dir() and not p.is_symlink():
                yield from files(p)
            elif p.is_file():
                yield p
            else:
                raise ValueError("unsupported checkpoint bundle entry")

    manifest = hashlib.sha256(b"icf-checkpoint-v1\0")
    count = 0
    for p in files(root):
        h = hashlib.sha256()
        with p.open("rb") as f:
            for block in iter(lambda: f.read(1 << 20), b""):
                h.update(block)
        manifest.update(p.relative_to(root).as_posix().encode() + b"\0" + h.digest())
        count += 1
    if not count:
        raise ValueError("empty checkpoint bundle")
    return manifest.hexdigest()


def wire_engine(name: str) -> str:
    # The licence register/trainer and the Go model registry use these two spellings.
    return "gptsovits" if name == "gpt_sovits" else name


def revision_for(engine: str, digest: str) -> str:
    """The same deployment revision for a reviewed, trained or mounted bundle."""
    base = {"cosyvoice": "0", "gptsovits": "v2", "voxcpm": "1"}.get(wire_engine(engine))
    if base is None:
        raise ValueError("unsupported checkpoint family")
    label = os.environ.get("ICF_CHECKPOINT_REVISION", "")
    prefix = base + ("." + label if label else "")
    if not re.fullmatch(r"[A-Za-z0-9._-]{1,128}", prefix):
        raise ValueError("invalid checkpoint revision label")
    return f"{prefix}+sha256.{digest}"


if __name__ == "__main__":
    # Run on the independently reviewed artifact; no network, GPU or ML imports.
    import argparse

    parser = argparse.ArgumentParser(description="Print the registry revision for a reviewed checkpoint directory")
    parser.add_argument("engine", choices=("cosyvoice", "gpt_sovits", "gptsovits", "voxcpm"))
    parser.add_argument("bundle", type=Path)
    args = parser.parse_args()
    print(revision_for(args.engine, bundle_sha256(args.bundle)))
