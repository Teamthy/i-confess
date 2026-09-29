-- Delivery encodings (spec section 26). The WAV master stays in storage_key;
-- derived web/mobile encodings (AAC, Opus, MP3) are recorded here as JSON
-- {format: {key, sha256, bytes, contentType}} so they can be signed for
-- delivery and purged together with the master under the delete policy.
ALTER TABLE voice_generations ADD COLUMN variants TEXT NOT NULL DEFAULT '{}';
