// Front-end defaults, mirroring the Go `config` package on the backend.
//
// Values come from Vite env vars at build time, with sensible fallbacks, so a
// deployment can tune them without touching code. This is also the natural home
// for future user-facing preferences (appearance, playback behaviour, ...).
//
//   VITE_DEFAULT_VOLUME  0..1, initial playback volume  (default 0.3)
//   VITE_AUTOPLAY        "true" to attempt playback on open (default off)

function clamp01(n) {
  return Math.min(1, Math.max(0, n));
}

function envNumber(value, fallback) {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

export const DEFAULT_VOLUME = clamp01(
  envNumber(import.meta.env.VITE_DEFAULT_VOLUME, 0.3),
);

// Off by default: the host should press play deliberately, which also gives the
// share capture a definite starting point.
export const AUTO_PLAY = import.meta.env.VITE_AUTOPLAY === "true";
