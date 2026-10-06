// Front-end defaults, mirroring the Go `config` package on the backend.
//
// Values come from Vite env vars at build time, with sensible fallbacks, so a
// deployment can tune them without touching code. This is also the natural home
// for future user-facing preferences (appearance, playback behaviour, ...).
//
//   VITE_DEFAULT_VOLUME  0..1, initial playback volume  (default 0.3)
//   VITE_AUTOPLAY        "true" to attempt playback on open (default off)
//   VITE_ICE_SERVERS     JSON array of RTCIceServer objects (default: STUN)

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

// --- WebRTC connectivity -------------------------------------------------
//
// STUN lets two peers discover their public addresses and connect directly.
// TURN relays the pairs that cannot connect directly (symmetric NAT, strict
// corporate or mobile firewalls); roughly 10-20% of pairs need it. TURN is the
// single biggest reliability lever for sharing, and it is a *server* — see
// README.md § "TURN relay" for how to obtain credentials.
//
// Default is public STUN only: fine on a LAN or friendly home networks.
const DEFAULT_ICE_SERVERS = [{ urls: "stun:stun.l.google.com:19302" }];

/**
 * Parse `VITE_ICE_SERVERS`, a JSON array of RTCIceServer objects, e.g.
 *   [{"urls":"stun:stun.l.google.com:19302"},
 *    {"urls":"turn:turn.example.com:3478","username":"u","credential":"p"}]
 * A malformed value falls back to STUN rather than breaking every connection.
 */
function parseIceServers(value) {
  if (!value) return DEFAULT_ICE_SERVERS;
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) && parsed.length > 0
      ? parsed
      : DEFAULT_ICE_SERVERS;
  } catch {
    console.warn("VITE_ICE_SERVERS is not valid JSON; using STUN only.");
    return DEFAULT_ICE_SERVERS;
  }
}

export const ICE_SERVERS = parseIceServers(import.meta.env.VITE_ICE_SERVERS);
