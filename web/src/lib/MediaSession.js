// MediaSession abstracts "a thing the player renders and controls" behind a
// single interface. The player UI only ever talks to this API, never to a
// <video> element or a WebRTC peer directly.
//
// This is the seam that makes the app server-deployable later: swapping the
// local/p2p implementation for a server-backed one is a new class, not a UI
// rewrite.
//
// Interface:
//   attach(el)             bind to a <video> element
//   play() / pause()       transport controls
//   seek(seconds)
//   getState()             { position, duration, playing }
//   on(event, handler)     'timeupdate' | 'ended' | 'playing' | 'pause'
//   destroy()

/**
 * LocalSession plays a file served by the host's Go backend. It is the only
 * implementation needed for the p2p MVP; a ServerSession (HLS/WebRTC SFU) can
 * implement the same interface later.
 */
export class LocalSession {
  constructor({ src }) {
    this.src = src;
    this.video = null;
    this.handlers = new Map();
  }

  attach(videoEl) {
    this.video = videoEl;
    this.video.src = this.src;
    this.video.preload = "metadata";

    // Bridge native media events into the session's own event names.
    this._bind("timeupdate", () => this._emit("timeupdate"));
    this._bind("ended", () => this._emit("ended"));
    this._bind("playing", () => this._emit("playing"));
    this._bind("pause", () => this._emit("pause"));
  }

  _bind(nativeEvent, fn) {
    if (!this.video) return;
    this.video.addEventListener(nativeEvent, fn);
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
    if (!this.video) {
      return { position: 0, duration: 0, playing: false, volume: 1, muted: false };
    }
    return {
      position: this.video.currentTime,
      duration: this.video.duration || 0,
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
      this.video.load();
    }
    this.handlers.clear();
  }
}