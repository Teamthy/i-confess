# iCONFESS voice engine — GPU inference worker.
#
# This image contains the worker and its own dependencies only. It deliberately
# contains NO TTS model and NO engine repository, for two reasons:
#
#   1. Licence. docs/model_licenses.json sets production_allowed=false for every
#      engine until counsel has verified the code AND the exact checkpoint.
#      Baking an uncleared model into a published image would defeat that gate
#      and put the weights somewhere they cannot be withdrawn from.
#   2. Size and reproducibility. Checkpoints are multi-gigabyte and change
#      independently of this code; they belong in the private bucket, mounted at
#      ICF_CHECKPOINT_ROOT, not in a layer.
#
# The engine repository (CosyVoice, etc.) and the checkpoints are supplied by
# the deployment: PYTHONPATH for the code, a volume for the weights.
#
#   docker build -t iconfess-voice-engine voice-engine
#   # GPU host, real inference:
#   docker build -t iconfess-voice-engine:cuda \
#     --build-arg BASE_IMAGE=nvidia/cuda:12.4.1-runtime-ubuntu22.04 voice-engine
#
# Local plumbing only, no GPU (emits a tone, never speech):
#   docker run --rm -e ICF_WORKER_DEV=1 -e ICF_ENV=development \
#     -p 8601:8601 iconfess-voice-engine --engine dev-tone

ARG BASE_IMAGE=python:3.11-slim
FROM ${BASE_IMAGE}

# ffmpeg is required for the delivery encodings (AAC/Opus/MP3). ca-certificates
# for any TLS the engine's own dependencies open; tini so the worker reaps
# children and exits on SIGTERM instead of ignoring it.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates tini \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install dependencies before copying code so the layer caches across edits.
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY --chown=10001:10001 icf_worker ./icf_worker

# Same uid/gid as server/Dockerfile so a shared volume keeps one owner.
RUN groupadd --gid 10001 iconfess \
 && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin iconfess \
 && mkdir -p /tmp/icf && chown -R 10001:10001 /app /tmp/icf

USER 10001:10001

# Bind to all interfaces so the container port is reachable; isolation is the
# NetworkPolicy's job, not the bind address's.
ENV ICF_WORKER_HOST=0.0.0.0 \
    ICF_WORKER_PORT=8601 \
    ICF_ENGINE=cosyvoice \
    PYTHONUNBUFFERED=1 \
    TMPDIR=/tmp/icf

EXPOSE 8601

# /v1/health authenticates like every other endpoint and returns 503 when the
# engine cannot load its model, which is what makes this a readiness signal
# rather than a liveness formality. The probe module sends the bearer token; an
# inline `python -c` that omits it gets 401 and the container never becomes
# healthy.
HEALTHCHECK --interval=30s --timeout=10s --start-period=120s --retries=3 \
  CMD ["python", "-m", "icf_worker.healthcheck"]

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "-m", "icf_worker.server"]
