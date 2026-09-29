"""Object storage shared with the Go API.

Keys are the same strings the API stores (e.g. voice-private/voices/...).

- ICF_STORAGE=local (default): keys map to files under ICF_STORAGE_ROOT,
  the same layout as the API's LocalStorage, for development.
- ICF_STORAGE=s3: an S3-compatible bucket (Cloudflare R2) via boto3, which is
  imported lazily. Downloads are cached under ICF_CACHE_DIR.

Keys are validated like the Go side: no traversal, no absolute paths, and
only the namespaces the worker legitimately touches.
"""

from __future__ import annotations

import os
import tempfile
from pathlib import Path

ALLOWED_PREFIXES = ("voice-private/", "audio/")


class StorageError(Exception):
    pass


def valid_key(key: str) -> bool:
    if not key or len(key) > 512 or ".." in key or key.startswith("/") or "//" in key:
        return False
    if any(c in key for c in "\\?#\x00"):
        return False
    return key.startswith(ALLOWED_PREFIXES)


class LocalStore:
    def __init__(self, root: str):
        self.root = Path(root).resolve()

    def _path(self, key: str) -> Path:
        if not valid_key(key):
            raise StorageError(f"invalid key {key!r}")
        p = (self.root / key).resolve()
        if self.root not in p.parents:
            raise StorageError("key escapes storage root")
        return p

    def local_path(self, key: str) -> Path:
        p = self._path(key)
        if not p.exists():
            raise StorageError(f"object not found: {key}")
        return p

    def put(self, key: str, data: bytes) -> None:
        p = self._path(key)
        p.parent.mkdir(parents=True, exist_ok=True)
        tmp = p.with_suffix(p.suffix + ".tmp")
        tmp.write_bytes(data)
        tmp.replace(p)  # atomic: never leave a truncated object

    def put_file(self, key: str, src: Path) -> None:
        self.put(key, Path(src).read_bytes())


class S3Store:  # pragma: no cover - needs a bucket
    def __init__(self):
        import boto3  # type: ignore

        self.bucket = os.environ["ICF_S3_BUCKET"]
        self.client = boto3.client(
            "s3",
            endpoint_url=os.environ.get("ICF_S3_ENDPOINT"),  # R2: https://<acct>.r2.cloudflarestorage.com
            aws_access_key_id=os.environ.get("ICF_S3_ACCESS_KEY_ID"),
            aws_secret_access_key=os.environ.get("ICF_S3_SECRET_ACCESS_KEY"),
            region_name=os.environ.get("ICF_S3_REGION", "auto"),
        )
        self.cache = Path(os.environ.get("ICF_CACHE_DIR", tempfile.gettempdir())) / "icf-cache"

    def local_path(self, key: str) -> Path:
        if not valid_key(key):
            raise StorageError(f"invalid key {key!r}")
        p = self.cache / key
        if not p.exists():
            p.parent.mkdir(parents=True, exist_ok=True)
            try:
                self.client.download_file(self.bucket, key, str(p))
            except Exception as e:  # noqa: BLE001
                raise StorageError(f"download {key}: {e}")
        return p

    def put(self, key: str, data: bytes) -> None:
        if not valid_key(key):
            raise StorageError(f"invalid key {key!r}")
        self.client.put_object(Bucket=self.bucket, Key=key, Body=data)

    def put_file(self, key: str, src: Path) -> None:
        if not valid_key(key):
            raise StorageError(f"invalid key {key!r}")
        self.client.upload_file(str(src), self.bucket, key)


_store = None


def get_store():
    global _store
    if _store is None:
        if os.environ.get("ICF_STORAGE", "local") == "s3":
            _store = S3Store()
        else:
            _store = LocalStore(os.environ.get("ICF_STORAGE_ROOT", "/data/objects"))
    return _store


def reset_store():
    """For tests: re-read configuration."""
    global _store
    _store = None
