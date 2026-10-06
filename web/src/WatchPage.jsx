import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { StreamSession } from "./lib/MediaSession";
import { ViewerSession } from "./lib/webrtc";
import { currentItem, decodeMessage } from "./lib/protocol";
import Player from "./Player";
import RoomSidebar from "./RoomSidebar";

const STATUS_TEXT = {
  idle: "Ready to join.",
  connecting: "Connecting to the host…",
  connected: "Connected — waiting for the host to start playing…",
  disconnected: "The connection dropped.",
  ended: "The host ended the session.",
  failed: "Could not connect.",
};

/** Stands in for the player until the host's stream arrives. */
function WaitingStage({ text }) {
  return (
    <div className="flex aspect-video w-full min-w-0 flex-1 flex-col items-center justify-center rounded-2xl border border-border bg-surface shadow-lg">
      <span className="h-8 w-8 animate-spin rounded-full border-2 border-border border-t-accent" />
      <p className="mt-4 max-w-xs text-center text-sm text-muted">{text}</p>
    </div>
  );
}

/**
 * WatchPage is the viewer side of a share link. It is deliberately independent
 * of the Go API: on a viewer's machine "localhost" is their own computer, so
 * everything it needs arrives over WebRTC — the playlist on the data channel,
 * the video as the host's live stream.
 */
export default function WatchPage() {
  const { roomId } = useParams();
  const [status, setStatus] = useState("idle");
  const [error, setError] = useState("");
  const [stream, setStream] = useState(null);
  // The host's shared state (its playlist, and which entry is on air).
  // Everything the viewer displays is derived from this.
  const [remote, setRemote] = useState(null);
  const viewerRef = useRef(null);

  // A session is created only once media actually arrives.
  const session = useMemo(
    () => (stream ? new StreamSession({ stream }) : null),
    [stream],
  );

  useEffect(() => () => viewerRef.current?.destroy(), []);

  const join = async () => {
    // A retry after a failed join must not leave the previous attempt's peer —
    // and its registration on the broker — behind.
    viewerRef.current?.destroy();
    viewerRef.current = null;

    setError("");
    setStatus("connecting");

    const viewer = new ViewerSession({
      onStatus: setStatus,
      onStream: setStream,
      // The data channel is the viewer's only source of truth about what it is
      // watching: it cannot ask the host's backend for anything.
      onMessage: (data) => {
        const state = decodeMessage(data);
        if (state) setRemote(state);
      },
      onError: (err) => setError(err.message),
    });
    viewerRef.current = viewer;

    try {
      await viewer.connect(roomId);
    } catch (err) {
      setStatus("failed");
      setError(err.message);
    }
  };

  const leave = () => {
    viewerRef.current?.destroy();
    viewerRef.current = null;
    setStream(null);
    setRemote(null);
    setStatus("ended");
  };
  const nowPlaying = currentItem(remote);
  // The room opens as soon as the data channel is up: the playlist arrives
  // before any video does, and a viewer should see where the queue stands while
  // it waits for the host to start.
  const joined = status === "connected" || Boolean(session);

  if (joined) {
    return (
      <div className="min-h-screen bg-canvas">
        <div className="mx-auto flex max-w-6xl flex-col gap-4 p-4 sm:px-6 lg:flex-row lg:items-start">
          <div className="flex min-w-0 justify-center lg:flex-1">
            {session ? (
              <Player
                mode="theatre"
                session={session}
                title={nowPlaying ? nowPlaying.filename : "Shared stream"}
                subtitle={
                  nowPlaying
                    ? `${remote.index + 1} of ${remote.items.length} · live from the host`
                    : "Live from the host"
                }
                onClose={leave}
              />
            ) : (
              <WaitingStage text={STATUS_TEXT[status]} />
            )}
          </div>

          <RoomSidebar
            role="viewer"
            items={remote?.items ?? []}
            index={remote?.index ?? -1}
            statusText={
              session
                ? "Live — you're watching the host's player."
                : STATUS_TEXT[status]
            }
          />
        </div>
      </div>
    );
  }

  const buttonLabel =
    status === "connecting"
      ? "Connecting…"
      : status === "idle"
        ? "Watch"
        : "Try again";

  return (
    <div className="flex min-h-full items-center justify-center px-4 py-16">
      <div className="w-full max-w-md rounded-2xl border border-border bg-surface p-6 text-center">
        <span className="mx-auto mb-4 grid h-12 w-12 place-items-center rounded-xl bg-accent text-lg font-bold text-white">
          L
        </span>

        <h1 className="text-lg font-semibold">You've been invited to watch</h1>
        <p className="mt-2 text-sm text-muted">{STATUS_TEXT[status]}</p>

        {error && (
          <p className="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">
            {error}
          </p>
        )}

        <button
          type="button"
          onClick={join}
          disabled={status === "connecting"}
          className="mt-5 w-full rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition hover:bg-accent-hover disabled:cursor-not-allowed disabled:opacity-60"
        >
          {buttonLabel}
        </button>

        <p className="mt-4 text-xs text-muted">
          Keep this tab open. Video travels directly between the host's browser
          and yours — nothing is uploaded to a server.
        </p>

        <Link
          to="/"
          className="mt-3 inline-block text-xs text-muted underline hover:text-foreground"
        >
          Go to my own library
        </Link>
      </div>
    </div>
  );
}

