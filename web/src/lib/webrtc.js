import Peer from "peerjs";

// --- Sharing strategy: PeerJS implementation -----------------------------
//
// This module is the "transport" seam for sharing. The React components only
// touch the two classes below, never PeerJS directly. Swapping in a
// server-hosted signalling hub later means adding a new implementation of the
// same shape, not rewriting the UI.
//
// How a session works (the host is authoritative):
//
//   host                                       viewer
//   ----                                       ------
//   Peer(id = room)                            Peer()
//   on("connection")          <-- data conn -- peer.connect(room)
//   read conn.peer
//   peer.call(viewerId, stream) -- media ---->  on("call") -> call.answer()
//                                               on("stream") -> <video>
//
// The viewer opens the data connection first, which tells the host the viewer's
// peer id. That avoids PeerJS's requirement to pass a stream when calling.
//
// Sync note: because the host sends a single live MediaStream, every viewer
// shares one clock automatically. No drift correction is needed.

// STUN lets peers discover their public address and connect directly. TURN is
// the relay fallback for the ~10-20% of pairs that cannot connect directly
// (symmetric NAT, some mobile networks). Add a TURN entry to `iceServers` to
// improve reliability; until then those pairs simply fail to connect.
const ICE_SERVERS = [
  { urls: "stun:stun.l.google.com:19302" },
  // { urls: "turn:your.turn.server:3478", username: "...", credential: "..." },
];

const PEER_OPTIONS = {
  debug: 1,
  config: { iceServers: ICE_SERVERS },
};

// Room ids are prefixed to avoid colliding with other users of the shared
// public PeerJS signalling server.
const ROOM_PREFIX = "lom-";
const ID_ATTEMPTS = 5;

function randomRoomId() {
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  return `${ROOM_PREFIX}${hex}`;
}

/**
 * Create a Peer, retrying if the randomly chosen id is already taken on the
 * shared signalling server.
 */
function createPeer(attempt = 1) {
  return new Promise((resolve, reject) => {
    const peer = new Peer(randomRoomId(), PEER_OPTIONS);

    const cleanup = () => {
      peer.off("open", onOpen);
      peer.off("error", onError);
    };

    const onOpen = (id) => {
      cleanup();
      resolve({ peer, id });
    };

    const onError = (err) => {
      cleanup();
      if (err?.type === "unavailable-id" && attempt < ID_ATTEMPTS) {
        peer.destroy();
        resolve(createPeer(attempt + 1));
        return;
      }
      peer.destroy();
      reject(err);
    };

    peer.on("open", onOpen);
    peer.on("error", onError);
  });
}

function describeError(err) {
  switch (err?.type) {
    case "peer-unavailable":
      return "That share link is no longer active.";
    case "network":
      return "Lost connection to the sharing service.";
    case "server-error":
      return "The sharing service is unavailable. Try again shortly.";
    case "browser-incompatible":
      return "This browser does not support peer-to-peer video.";
    case "unavailable-id":
      return "Could not allocate a session. Try again.";
    default:
      return err?.message || "Something went wrong with the connection.";
  }
}

/**
 * HostSession owns the room. It waits for viewers to connect, then pushes the
 * captured MediaStream to each of them.
 */
export class HostSession {
  constructor({ onViewersChange, onMessage, onError } = {}) {
    this.onViewersChange = onViewersChange || (() => {});
    this.onMessage = onMessage || (() => {});
    this.onError = onError || (() => {});

    this.peer = null;
    this.roomId = null;
    this.stream = null;
    this.connections = new Map(); // viewerId -> DataConnection
    this.calls = new Map(); // viewerId -> MediaConnection
  }

  /** Creates the room and resolves with its id. */
  async start() {
    const { peer, id } = await createPeer();
    this.peer = peer;
    this.roomId = id;

    // Runtime errors (after opening) are reported but not fatal.
    peer.on("error", (err) => this.onError(new Error(describeError(err))));

    // A viewer opens a data connection first; that reveals their peer id.
    peer.on("connection", (conn) => this._handleConnection(conn));

    return this.roomId;
  }

  _handleConnection(conn) {
    conn.on("open", () => {
      this.connections.set(conn.peer, conn);
      this.onViewersChange(this.connections.size);

      // A viewer joining mid-stream should start watching immediately.
      if (this.stream) this._callViewer(conn.peer, this.stream);
    });

    conn.on("data", (data) => this.onMessage(conn.peer, data));
    conn.on("close", () => this._dropViewer(conn.peer));
    conn.on("error", () => this._dropViewer(conn.peer));
  }

  _callViewer(viewerId, stream) {
    if (this.calls.has(viewerId)) return;

    const call = this.peer.call(viewerId, stream);
    if (!call) return;

    this.calls.set(viewerId, call);
    call.on("close", () => this.calls.delete(viewerId));
    call.on("error", () => this.calls.delete(viewerId));
  }

  /** Start (or replace) the stream sent to every viewer. */
  publishStream(stream) {
    this.stream = stream;
    this.connections.forEach((_conn, viewerId) => {
      this._callViewer(viewerId, stream);
    });
  }

  /** Send a control message to every connected viewer. */
  broadcast(data) {
    this.connections.forEach((conn) => {
      if (conn.open) conn.send(data);
    });
  }

  _dropViewer(viewerId) {
    this.connections.delete(viewerId);

    const call = this.calls.get(viewerId);
    if (call) {
      call.close();
      this.calls.delete(viewerId);
    }

    this.onViewersChange(this.connections.size);
  }

  get viewerCount() {
    return this.connections.size;
  }

  destroy() {
    this.connections.forEach((conn) => conn.close());
    this.connections.clear();

    this.calls.forEach((call) => call.close());
    this.calls.clear();

    // The captured stream's tracks are owned by the host's own <video> element,
    // so we deliberately do not stop them here.
    this.peer?.destroy();
    this.peer = null;
  }
}

/**
 * ViewerSession joins an existing room. It connects the data channel (which
 * clues the host in), then waits to be called with the host's stream.
 */
export class ViewerSession {
  constructor({ onStream, onStatus, onMessage, onError } = {}) {
    this.onStream = onStream || (() => {});
    this.onStatus = onStatus || (() => {});
    this.onMessage = onMessage || (() => {});
    this.onError = onError || (() => {});

    this.peer = null;
    this.conn = null;
    this.call = null;
    this.stream = null;
    this.closed = false;
  }

  async connect(roomId) {
    this.onStatus("connecting");

    const { peer } = await createPeer();
    this.peer = peer;

    if (this.closed) {
      peer.destroy();
      return;
    }

    peer.on("call", (call) => this._handleCall(call));
    peer.on("error", (err) => {
      this.onStatus("failed");
      this.onError(new Error(describeError(err)));
    });

    const conn = peer.connect(roomId, { reliable: true });
    this.conn = conn;

    conn.on("open", () => this.onStatus("connected"));
    conn.on("data", (data) => this.onMessage(data));
    conn.on("close", () => this.onStatus("disconnected"));
    conn.on("error", (err) => this.onError(new Error(describeError(err))));
  }

  _handleCall(call) {
    this.call = call;

    // Receive-only: the viewer sends nothing back up.
    call.answer();

    call.on("stream", (stream) => {
      this.stream = stream;
      this.onStream(stream);
    });

    call.on("close", () => this.onStatus("ended"));
    call.on("error", (err) => this.onError(new Error(describeError(err))));
  }

  send(data) {
    if (this.conn?.open) this.conn.send(data);
  }

  destroy() {
    this.closed = true;
    this.call?.close();
    this.conn?.close();
    this.peer?.destroy();
    this.peer = null;
    this.conn = null;
    this.call = null;
  }
}

