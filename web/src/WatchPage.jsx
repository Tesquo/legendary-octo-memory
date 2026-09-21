import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { StreamSession } from "./lib/MediaSession";
import { ViewerSession } from "./lib/webrtc";
import Player from "./Player";

const STATUS_TEXT = {
  idle: "Ready to join.",
  connecting: "Connecting to the host…",
  connected: "Connected — waiting for the host to start playing…",
  disconnected: "The connection dropped.",
  ended: "The host stopped sharing.",
  failed: "Could not connect.",
};

/**
 * WatchPage is the viewer side of a share link. It is deliberately independent
 * of the Go API: on a viewer's machine "localhost" is their own computer, so
 * everything it needs arrives over WebRTC.
 */
export default function WatchPage() {
  const { roomId } = useParams();
  const [status, setStatus] = useState("idle");
  const [error, setError] = useState("");
  const [stream, setStream] = useState(null);
  const viewerRef = useRef(null);

  // A session is created only once media actually arrives.
  const session = useMemo(
    () => (stream ? new StreamSession({ stream }) : null),
    [stream],
  );

  useEffect(() => () => viewerRef.current?.destroy(), []);

  const join = async () => {
    setError("");
    setStatus("connecting");

    const viewer = new ViewerSession({
      onStatus: setStatus,
      onStream: setStream,
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
    setStatus("ended");
  };

  if (session) {
    return (
      <div className="min-h-screen bg-canvas">
        <Player
          mode="page"
          session={session}
          title="Shared stream"
          subtitle="Live from the host"
          onClose={leave}
        />
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
