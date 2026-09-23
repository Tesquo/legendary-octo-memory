// Wire format for the WebRTC data channel.
//
// The viewer must never call the Go API (on their machine "localhost" is their
// own computer), so everything a viewer displays has to be pushed to it by the
// host. This module is the single definition of what travels over the data
// channel: versioned, validated, and deliberately tiny.
//
// Messages are plain JSON objects, so the transport (`webrtc.js`) stays ignorant
// of their meaning and a server-hosted signalling hub can replace it later.
//
//   { v: 1, type: "state", index: 2, items: [ { id, filename, duration } ] }
//
// A single message type carries the whole shared world: the host-owned queue and
// which entry is playing right now. It is sent whenever any of that changes, and
// once to every viewer as it connects, so a late joiner is correct immediately.
// Everything the viewer shows is derived from it, rather than kept in a second
// message that could fall out of sync.

export const PROTOCOL_VERSION = 1;

export const MESSAGE_STATE = "state";

/**
 * Reduce a library item to the fields a peer is allowed to see. The host's
 * filesystem layout, media paths and API URLs are deliberately not shared.
 */
function publicItem(item) {
  return {
    id: String(item.id ?? ""),
    filename: String(item.filename ?? ""),
    duration: Number.isFinite(item.duration) ? item.duration : 0,
  };
}

/**
 * Build the shared-state message. `index` is the queue position being played, or
 * -1 when whatever is playing did not come from the queue.
 */
export function stateMessage({ items = [], index = -1 } = {}) {
  return {
    v: PROTOCOL_VERSION,
    type: MESSAGE_STATE,
    index: Number.isInteger(index) ? index : -1,
    items: items.map(publicItem),
  };
}

/**
 * Validate an inbound frame. Returns the shared state, or null for anything this
 * version does not understand, so a malformed or future message is ignored
 * rather than breaking the viewer.
 */
export function decodeMessage(data) {
  if (!data || typeof data !== "object") return null;
  if (data.v !== PROTOCOL_VERSION) return null;
  if (data.type !== MESSAGE_STATE) return null;
  if (!Array.isArray(data.items)) return null;

  return {
    index: Number.isInteger(data.index) ? data.index : -1,
    items: data.items.map(publicItem),
  };
}

/** The queue entry being played, or null when there is none. */
export function currentItem(state) {
  if (!state || !Array.isArray(state.items)) return null;
  return state.items[state.index] || null;
}
