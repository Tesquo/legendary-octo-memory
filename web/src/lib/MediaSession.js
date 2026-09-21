import { DEFAULT_VOLUME } from "./config";

// MediaSession abstracts "a thing the player renders and controls" behind a
// single interface. The player UI only ever talks to this API, never to a
// <video> element or a WebRTC peer directly.
//
// This is the seam that makes the app shareable and server-deployable later:
// swapping the local/p2p implementation for a server-backed one is a new class,
// not a UI rewrite.
//
// Interface:
//   attach(el)                bind to a <video> element
//   play() / pause()          transport controls
//   seek(seconds)             no-op when canSeek is false
//   getState()                { position, duration, playing, volume, muted }
//   canSeek                   false for live streams
//   on(event, handler)        'timeupdate' | 'playing' | 'pause' | 'ended' | 'volumechange'
//   destroy()

const EMPTY_STATE = {
  position: 0,
  duration: 0,
  playing: false,
  volume: DEFAULT_VOLUME,
  muted: false,
};

/**
 * BaseSession holds the behaviour every session shares: event subscription,
 * volume/mute handling, and reading transport state off the bound <video>.
 * Subclasses only decide *what* is attached to the element.
 */
class BaseSession {
  constructor() {
    this.video = null;
    this.handlers = new Map();
  }

  attach(videoEl) {
    this.video = videoEl;
    // Start at a comfortable level rather than full volume.
    this.video.volume = DEFAULT_VOLUME;
  }

  _bind(nativeEvent, fn) {
    if (!this.video) return;
    this.video.addEventListener(nativeEvent, fn);
  }

  /** Bridge the native media events the UI cares about into session events. */
  _bindCommonEvents() {
    this._bind("timeupdate", () => this._emit("timeupdate"));
    this._bind("ended", () => this._emit("ended"));
    this._bind("playing", () => this._emit("playing"));
    this._bind("pause", () => this._emit("pause"));
  }

  play() {
    return this.video?.play();
  }

  pause() {
    this.video?.pause();
  }

  seek(seconds) {
    if (this.video) this.video.currentTime = seconds;
  }

  getState() {
    if (!this.video) return { ...EMPTY_STATE };
    return {
      position: this.video.currentTime,
      // Live streams report an infinite duration; normalise to 0.
      duration: Number.isFinite(this.video.duration) ? this.video.duration : 0,
      playing: !this.video.paused,
      volume: this.video.volume,
      muted: this.video.muted,
    };
  }

  setVolume(value) {
    if (!this.video) return;
    this.video.volume = Math.min(1, Math.max(0, value));
    if (this.video.volume > 0) this.video.muted = false;
    this._emit("volumechange");
  }

  setMuted(muted) {
    if (!this.video) return;
    this.video.muted = muted;
    this._emit("volumechange");
  }

  on(event, handler) {
    if (!this.handlers.has(event)) this.handlers.set(event, new Set());
    this.handlers.get(event).add(handler);
    return () => this.handlers.get(event)?.delete(handler);
  }

  _emit(event) {
    this.handlers.get(event)?.forEach((fn) => fn(this.getState()));
  }

  destroy() {
    if (this.video) {
      this.video.pause();
      this.video.removeAttribute("src");
      this.video.srcObject = null;
      this.video.load();
    }
    this.handlers.clear();
  }

  /** Whether the transport allows seeking. Live streams override this. */
  get canSeek() {
    return true;
  }
}

/**
 * LocalSession plays a file served by the host's Go backend. The host uses this;
 * the file is seekable and fully controllable.
 */
export class LocalSession extends BaseSession {
  constructor({ src }) {
    super();
    this.src = src;
  }

  attach(videoEl) {
    super.attach(videoEl);
    this.video.srcObject = null;
    this.video.src = this.src;
    this.video.preload = "metadata";
    this._bindCommonEvents();
  }
}

/**
 * StreamSession plays a remote MediaStream received over WebRTC. The viewer
 * uses this. It has no timeline to seek, so `canSeek` is false and the player
 * hides its scrubber.
 */
export class StreamSession extends BaseSession {
  constructor({ stream }) {
    super();
    this.stream = stream;
  }

  attach(videoEl) {
    super.attach(videoEl);
    this.video.removeAttribute("src");
    this.video.srcObject = this.stream;
    // A live stream can autoplay muted without a gesture; the viewer page shows
    // an explicit "click to watch" affordance to satisfy the audio policy.
    this.video.autoplay = true;
    this._bindCommonEvents();
  }

  seek() {
    // Live streams cannot be seeked. Intentionally a no-op.
  }

  get canSeek() {
    return false;
  }
}
