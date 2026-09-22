# iCONFESS web platform

Next.js 15 + React 19 + TypeScript. Marketing site (77 routes) plus a functional
app shell: 39 categories, 78 reviewed confessions, sessions, voices, journal,
memory practice, community review flow, premium gating, PWA, dark/contrast/
dyslexia themes, WCAG-minded.

## Run

```bash
cd web
npm install --no-audit --no-fund
npm run build
npm run start   # serves on :3000, binds 0.0.0.0
```

Dev: `npx next dev -H 0.0.0.0 -p 3000`.

## Notes

- No heavy UI deps: hand-rolled CSS (`app/globals.css`), no tailwind/framer.
- Audio is real: Web Speech narration engine with queue, repeat, sleep timer,
  whisper check, ambient bed (`lib/ui.tsx`).
- State persists to localStorage (`lib/store.ts`); demo verification code `000000`.
- Seeds: `lib/cats.json` (39 categories), `lib/confs.json` (78 confessions).
- Brand: official logos in `public/assets/logo-*.jpg`; tagline
  "SPEAK IT. BELIEVE IT. LIVE IT."
