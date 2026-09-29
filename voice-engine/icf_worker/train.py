"""Fine-tuning runs, driven by the Go API through submit/poll/cancel.

The worker does not implement training itself. Each engine ships its own
training scripts, and wrapping them faithfully is safer than reimplementing
them. An operator configures a command template per engine:

    ICF_TRAIN_CMD_GPT_SOVITS="python /opt/icf/train_gpt_sovits.py \
        --manifest {manifest} --out {out} --base {base_model} --hparams {hparams}"

The command receives a manifest (JSONL: local audio path, verified
transcript, split) and must write its checkpoint files into {out}. It may
print lines of the form "ICF_PROGRESS <0..1> loss=<float>" to report
progress. When it exits 0, the worker hashes and uploads everything in {out}
under the run's checkpoint key and reports COMPLETED. Promotion is never
automatic: the API registers the result as a candidate that must pass the
evaluation gate and get human approval.

Only train-split entries are given to the trainer as training data.
Validation entries are passed separately, and test entries are withheld
entirely so the golden-set evaluation stays honest.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import shlex
import subprocess
import tempfile
import threading
import time
from pathlib import Path

from . import licenses
from .storage import get_store

PROGRESS_RE = re.compile(r"ICF_PROGRESS\s+([0-9.]+)(?:\s+loss=([0-9.eE+-]+))?")


class TrainingError(Exception):
    def __init__(self, cls: str, message: str):
        super().__init__(message)
        self.cls = cls


class Run:
    def __init__(self, run_id: str):
        self.id = run_id
        self.status = "RUNNING"
        self.progress: float | None = 0.0
        self.loss: float | None = None
        self.error = ""
        self.checkpoint_key = ""
        self.checkpoint_sha256 = ""
        self.started = time.time()
        self.duration = 0
        self.proc: subprocess.Popen | None = None
        self.cancelled = False

    def view(self) -> dict:
        return {"status": self.status, "progress": self.progress, "loss": self.loss, "error": self.error,
                "checkpoint_key": self.checkpoint_key, "checkpoint_sha256": self.checkpoint_sha256,
                "hardware": os.environ.get("ICF_HARDWARE", ""), "duration_seconds": self.duration}


class TrainingManager:
    def __init__(self, workdir: str | None = None):
        self.workdir = Path(workdir or os.environ.get("ICF_TRAIN_DIR", tempfile.gettempdir())) / "icf-train"
        self.runs: dict[str, Run] = {}
        self.lock = threading.Lock()

    def _command(self, engine: str) -> str:
        cmd = os.environ.get("ICF_TRAIN_CMD_" + engine.upper().replace("-", "_"))
        if not cmd:
            raise TrainingError("model", f"training for engine {engine!r} is not configured on this worker")
        return cmd

    def submit(self, req: dict) -> Run:
        run_id = req.get("run_id") or ""
        if not re.fullmatch(r"[A-Za-z0-9_-]{1,80}", run_id):
            raise TrainingError("permanent", "invalid run_id")
        with self.lock:
            if run_id in self.runs:  # idempotent resubmission
                return self.runs[run_id]
        engine = req.get("engine", "")
        try:
            licenses.check(engine, "training")
        except licenses.LicenseError as e:
            # SystemExit subclass at startup; inside a request it is a refusal.
            raise TrainingError("permanent", str(e))
        cmd = self._command(engine)
        entries = req.get("entries") or []
        train = [e for e in entries if e.get("split") == "train"]
        val = [e for e in entries if e.get("split") == "validation"]
        if not train:
            raise TrainingError("content", "dataset has no training entries")
        store = get_store()
        rdir = self.workdir / run_id
        out = rdir / "out"
        out.mkdir(parents=True, exist_ok=True)

        def write(name, items):
            p = rdir / name
            with p.open("w") as f:
                for e in items:
                    f.write(json.dumps({"audio": str(store.local_path(e["audio_key"])), "text": e["transcript"],
                                        "style": e.get("style", ""), "segment_id": e["segment_id"]}) + "\n")
            return p

        manifest, valman = write("train.jsonl", train), write("validation.jsonl", val)
        hp = rdir / "hparams.json"
        hp.write_text(json.dumps(req.get("hyperparameters") or {}))
        argv = [a.format(manifest=manifest, validation=valman, out=out, base_model=req.get("base_model", ""), hparams=hp)
                for a in shlex.split(cmd)]
        run = Run(run_id)
        with self.lock:
            self.runs[run_id] = run
        threading.Thread(target=self._execute, args=(run, argv, out, req.get("checkpoint_key", "")), daemon=True).start()
        return run

    def _execute(self, run: Run, argv: list[str], out: Path, ckpt_prefix: str):
        try:
            run.proc = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            tail: list[str] = []
            for line in run.proc.stdout:  # type: ignore[union-attr]
                tail = (tail + [line.rstrip()])[-20:]
                m = PROGRESS_RE.search(line)
                if m:
                    run.progress = min(float(m.group(1)), 1.0)
                    if m.group(2):
                        run.loss = float(m.group(2))
            code = run.proc.wait()
            run.duration = int(time.time() - run.started)
            if run.cancelled:
                run.status, run.error = "FAILED", "cancelled"
                return
            if code != 0:
                run.status, run.error = "FAILED", f"trainer exited {code}: " + " | ".join(tail[-3:])
                return
            files = sorted(p for p in out.rglob("*") if p.is_file())
            if not files:
                run.status, run.error = "FAILED", "trainer produced no checkpoint files"
                return
            h = hashlib.sha256()
            store = get_store()
            for p in files:
                rel = p.relative_to(out).as_posix()
                h.update(rel.encode())
                h.update(p.read_bytes())
                store.put_file(f"{ckpt_prefix.rstrip('/')}/{rel}", p)
            run.checkpoint_key = ckpt_prefix.rstrip("/") + "/" + files[0].relative_to(out).as_posix()
            run.checkpoint_sha256 = h.hexdigest()
            run.progress, run.status = 1.0, "COMPLETED"
        except Exception as e:  # noqa: BLE001
            run.status, run.error = "FAILED", f"{type(e).__name__}: {e}"

    def get(self, run_id: str) -> Run | None:
        return self.runs.get(run_id)

    def cancel(self, run_id: str) -> bool:
        run = self.runs.get(run_id)
        if not run:
            return False
        run.cancelled = True
        if run.proc and run.proc.poll() is None:
            run.proc.terminate()
        return True
