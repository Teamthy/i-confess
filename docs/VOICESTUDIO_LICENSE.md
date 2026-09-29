# VoiceStudio: usage boundary

The build brief describes VoiceStudio as AGPL-licensed. Until that is
verified, we treat it as AGPL-3.0, which means:

- **Allowed:** running it on engineer workstations or an isolated benchmark
  box to compare engines against the golden set, with rights-cleared
  reference audio only.
- **Not allowed:** copying its source into this repository; importing it from
  `server/` or `voice-engine/icf_worker/`; or serving it to users over a
  network, since AGPL §13 would then require releasing the corresponding
  source.

The Go adapter `voiceengine.NewVoiceStudioProvider` exists only so benchmark
runs go through the same wire protocol. The registry refuses to register it
in production (`NewRegistry(production=true)`), and `docs/model_licenses.json`
marks it `production_allowed: false`.
