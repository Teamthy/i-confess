# Voice engine licences

The machine-readable register is [`model_licenses.json`](model_licenses.json).
This page explains how to use it. **It is not legal advice**; have counsel
confirm every entry before commercial launch.

## Rules

1. **Code and weights are separate.** An Apache-2.0 repository can ship
   weights under a non-commercial licence. Verify the exact checkpoint you
   deploy, identified by its SHA-256.
2. **`null` means unverified.** No field is filled in from memory or
   assumption. `commercial_use_verified` stays `null` until someone reads the
   licence text for that commit/checkpoint and signs off (`verified_by`,
   `verified_at`).
3. **Promotion is blocked by review.** `voice_models.license_reviewed` must be
   true before `voiceengine.Promote` accepts a model. Setting it is an audited
   admin action; it should only be set once the matching JSON entry is
   verified.
4. **Engine licence ≠ voice rights.** A permissively licensed engine grants
   nothing about a minister's voice. Voice use is governed separately by the
   rights registry (`voice_rights`, `voicegov`).
5. **No new engine without an entry.** Adding an adapter means adding a
   register entry in the same change.

## Current status

| Engine | Code licence (reported) | Weights | Commercial use verified | Production |
|---|---|---|---|---|
| CosyVoice | Apache-2.0 | not reviewed | no | blocked |
| GPT-SoVITS | MIT | not reviewed | no | blocked |
| VoxCPM | not reviewed | not reviewed | no | blocked |
| VoiceStudio | AGPL-3.0 (per brief) | — | n/a | never, dev tool only |

"Reported" means the licence the upstream repository displays. Nobody has yet
verified it at a pinned commit.
