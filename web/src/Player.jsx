import { useCallback, useEffect, useRef, useState } from "react";
import { AUTO_PLAY } from "./lib/config";

function formatTime(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  const mm = h > 0 ? m.toString().padStart(2, "0") : m;
  const ss = s.toString().padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

function Icon({ path, className = "h-5 w-5" }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      className={className}
      aria-hidden="true"
    >
      <path d={path} />
    </svg>
  );
}

const ICONS = {
  play: "M8 5v14l11-7z",
  pause: "M6 5h4v14H6zM14 5h4v14h-4z",
  volume:
    "M3 10v4h4l5 5V5L7 10H3zm13.5 2a4.5 4.5 0 0 0-2.5-4v8a4.5 4.5 0 0 0 2.5-4z",
  mute: "M3 10v4h4l5 5V5L7 10H3zm15.5 1.5L16.5 9.5 15 11l2 2-2 2 1.5 1.5 2-2 2 2 1.5-1.5-2-2 2-2-1.5-1.5-2 2z",
  fullscreen:
    "M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z",
  exitFullscreen:
    "M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z",
  close: "M18.3 5.71 12 12l6.3 6.29-1.41 1.42L10.59 13.4 4.3 19.71 2.89 18.3 9.18 12 2.89 5.71 4.3 4.29l6.29 6.3 6.3-6.3z",
};

/**
 * Player renders whatever MediaSession it is given, with fully custom controls.
 * Native controls are disabled so the transport is consistent across browsers.
 *
 * The session is owned by the caller, so it can be shared with other concerns
 * (such as capturing it for WebRTC). Player only attaches it to the <video>
 * element and subscribes to its events.
 *
 * Props:
 *   session       a MediaSession instance (LocalSession or StreamSession)
 *   title         primary label shown in the footer
 *   subtitle      secondary label (codecs, resolution, ...)
 *   onClose       called when the user dismisses the player
 *   onVideoReady  receives the <video> element once the session is attached,
 *                 which is how the host captures it for sharing
 *   autoPlay      attempt playback on attach; defaults to the configured value
 *                 (off, so the host presses play deliberately)
 *   mode          "modal" (host, a centred card) or "page" (viewer, fills the
 *                 viewport so the video is the focus of the page)
 *   toolbar       optional node rendered at the right of the footer, used for
 *                 the share controls so they sit inside the player chrome
 */
export default function Player({
  session,
  title,
  subtitle,
  onClose,
  onVideoReady,
  autoPlay = AUTO_PLAY,
  mode = "modal",
  toolbar,
}) {
  const containerRef = useRef(null);
  const videoRef = useRef(null);
  const sessionRef = useRef(null);
  const hideTimer = useRef(null);

  // Held in a ref so a changing callback identity never re-runs the attach
  // effect (which would re-bind the media element on every parent render).
  const videoReadyRef = useRef(onVideoReady);

  useEffect(() => {
    videoReadyRef.current = onVideoReady;
  }, [onVideoReady]);

  const [state, setState] = useState({
    position: 0,
    duration: 0,
    playing: false,
    volume: 1,
    muted: false,
  });
  const [error, setError] = useState("");
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [controlsVisible, setControlsVisible] = useState(true);

  useEffect(() => {
    if (!session) return;

    sessionRef.current = session;

    const videoEl = videoRef.current;
    if (videoEl) session.attach(videoEl);

    const sync = (s) => setState(s);
    const offs = [
      session.on("timeupdate", sync),
      session.on("playing", sync),
      session.on("pause", sync),
      session.on("ended", sync),
      session.on("volumechange", sync),
    ];

    const onError = () => setError("This stream could not be played.");
    videoEl?.addEventListener("error", onError);

    // Let the owner grab the element, e.g. to captureStream() it for sharing.
    videoReadyRef.current?.(videoEl);

    // The caller opened the player from a click, so playback is permitted here.
    // A rejection simply leaves the click-to-play overlay in place.
    if (autoPlay) session.play()?.catch(() => {});

    return () => {
      offs.forEach((off) => off?.());
      videoEl?.removeEventListener("error", onError);
      sessionRef.current = null;
      // The session is owned by the caller, so it is intentionally not
      // destroyed here.
    };
  }, [session, autoPlay]);

  const togglePlay = useCallback(() => {
    const s = sessionRef.current;
    if (!s) return;
    if (s.getState().playing) s.pause();
    else s.play();
  }, []);

  const seekBy = useCallback((delta) => {
    const s = sessionRef.current;
    if (!s) return;
    const { position, duration } = s.getState();
    const next = Math.min(Math.max(0, position + delta), duration || 0);
    s.seek(next);
  }, []);

  const toggleFullscreen = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    if (document.fullscreenElement) document.exitFullscreen();
    else el.requestFullscreen?.();
  }, []);

  const toggleMute = useCallback(() => {
    const s = sessionRef.current;
    if (!s) return;
    s.setMuted(!s.getState().muted);
  }, []);

  // Reflect fullscreen changes (which can also be triggered via Esc).
  useEffect(() => {
    const onChange = () => setIsFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  // Keyboard shortcuts. Ignored while typing in an input.
  useEffect(() => {
    if (!session) return;
    const onKey = (e) => {
      const tag = e.target?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;

      switch (e.key.toLowerCase()) {
        case " ":
        case "k":
          e.preventDefault();
          togglePlay();
          break;
        case "arrowright":
          seekBy(5);
          break;
        case "arrowleft":
          seekBy(-5);
          break;
        case "f":
          toggleFullscreen();
          break;
        case "m":
          toggleMute();
          break;
        default:
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [session, togglePlay, seekBy, toggleFullscreen, toggleMute]);

  // Auto-hide controls after inactivity while playing.
  const revealControls = useCallback(() => {
    setControlsVisible(true);
    clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => {
      if (sessionRef.current?.getState().playing) setControlsVisible(false);
    }, 2500);
  }, []);

  useEffect(() => () => clearTimeout(hideTimer.current), []);

  if (!session) return null;

  const canSeek = session.canSeek;
  const isPage = mode === "page";
  const pct = state.duration ? (state.position / state.duration) * 100 : 0;
  const volumePct = state.muted ? 0 : state.volume * 100;

  return (
    <div
      className={
        isPage
          ? "flex min-h-full w-full items-center justify-center bg-canvas p-4"
          : "fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm"
      }
    >
      <div
        ref={containerRef}
        className={
          isPage
            ? "relative w-full overflow-hidden rounded-2xl border border-border bg-surface"
            : "relative w-full max-w-6xl overflow-hidden rounded-2xl border border-border bg-surface shadow-2xl"
        }
        onMouseMove={revealControls}
        onMouseLeave={() => state.playing && setControlsVisible(false)}
      >
        <button
          type="button"
          onClick={onClose}
          className="absolute right-3 top-3 z-20 rounded-full bg-black/50 p-2 text-white/80 transition hover:bg-black/70 hover:text-white"
          aria-label="Close player"
        >
          <Icon path={ICONS.close} />
        </button>

        <div className="relative bg-black">
          <video
            ref={videoRef}
            playsInline
            onClick={togglePlay}
            className={`w-full bg-black ${
              isPage ? "max-h-[80vh]" : "max-h-[75vh]"
            }`}
          />

          {/* Click-to-play overlay when paused. */}
          {!state.playing && !error && (
            <button
              type="button"
              onClick={togglePlay}
              className="absolute inset-0 flex items-center justify-center bg-black/20 transition hover:bg-black/30"
              aria-label="Play"
            >
              <span className="rounded-full bg-white/90 p-5 text-canvas shadow-lg">
                <Icon path={ICONS.play} className="h-8 w-8" />
              </span>
            </button>
          )}
        </div>

        {error ? (
          <p className="px-4 py-3 text-sm text-red-400">{error}</p>
        ) : (
          <div
            className={`absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 to-transparent px-4 pb-3 pt-10 transition-opacity duration-300 ${
              controlsVisible ? "opacity-100" : "opacity-0"
            }`}
          >
            {/* A live stream has no timeline, so show a badge instead. */}
            {canSeek ? (
              <div className="relative mb-3 h-1.5 w-full">
                <div className="absolute top-0 h-1.5 w-full rounded-full bg-white/25" />
                <div
                  className="absolute top-0 h-1.5 rounded-full bg-accent"
                  style={{ width: `${pct}%` }}
                />
                <input
                  type="range"
                  min={0}
                  max={state.duration || 0}
                  step={0.1}
                  value={state.position}
                  onChange={(e) =>
                    sessionRef.current?.seek(Number(e.target.value))
                  }
                  className="absolute inset-0 h-1.5 w-full opacity-0"
                  aria-label="Seek"
                />
              </div>
            ) : (
              <div className="mb-3">
                <span className="inline-flex items-center gap-1.5 rounded-full bg-red-500/90 px-2 py-0.5 text-[11px] font-semibold tracking-wide text-white">
                  <span className="h-1.5 w-1.5 rounded-full bg-white" />
                  LIVE
                </span>
              </div>
            )}

            <div className="flex items-center gap-3 text-white">
              <button
                type="button"
                onClick={togglePlay}
                aria-label={state.playing ? "Pause" : "Play"}
                className="transition hover:text-accent"
              >
                <Icon path={state.playing ? ICONS.pause : ICONS.play} />
              </button>

              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={toggleMute}
                  aria-label={state.muted ? "Unmute" : "Mute"}
                  className="transition hover:text-accent"
                >
                  <Icon path={state.muted ? ICONS.mute : ICONS.volume} />
                </button>
                <div className="relative hidden h-1.5 w-20 sm:block">
                  <div className="absolute top-0 h-1.5 w-full rounded-full bg-white/25" />
                  <div
                    className="absolute top-0 h-1.5 rounded-full bg-white"
                    style={{ width: `${volumePct}%` }}
                  />
                  <input
                    type="range"
                    min={0}
                    max={1}
                    step={0.01}
                    value={state.muted ? 0 : state.volume}
                    onChange={(e) =>
                      sessionRef.current?.setVolume(Number(e.target.value))
                    }
                    className="absolute inset-0 h-1.5 w-full opacity-0"
                    aria-label="Volume"
                  />
                </div>
              </div>

              <span className="text-xs tabular-nums text-white/80">
                {formatTime(state.position)} / {formatTime(state.duration)}
              </span>

              <div className="ml-auto flex items-center gap-3">
                <button
                  type="button"
                  onClick={toggleFullscreen}
                  aria-label="Toggle fullscreen"
                  className="transition hover:text-accent"
                >
                  <Icon
                    path={
                      isFullscreen ? ICONS.exitFullscreen : ICONS.fullscreen
                    }
                  />
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Title bar sits below the video, outside the controls overlay. */}
        <div className="flex items-center justify-between gap-4 border-t border-border px-4 py-3">
          <div className="flex min-w-0 items-center gap-3">
            <h2 className="truncate text-sm font-medium" title={title}>
              {title}
            </h2>
            {subtitle && (
              <span className="hidden shrink-0 text-xs text-muted sm:block">
                {subtitle}
              </span>
            )}
          </div>
          {toolbar}
        </div>
      </div>
    </div>
  );
}