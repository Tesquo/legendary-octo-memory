import { useCallback, useEffect, useRef, useState } from "react";
import {
  listMedia,
  uploadMedia,
  reprocess,
  deleteMedia,
  thumbnailUrl,
  playUrl,
} from "./lib/api";
import { LocalSession } from "./lib/MediaSession";
import { HostSession } from "./lib/webrtc";
import { stateMessage } from "./lib/protocol";
import { useProgress } from "./lib/useProgress";
import Player from "./Player";
import ShareToolbar from "./ShareToolbar";
import QueueBar from "./QueueBar";
import QueueList from "./QueueList";

/** Build the viewer link for a room, honouring any sub-path deployment. */
function buildShareUrl(roomId) {
  const { origin, pathname } = window.location;
  const base = pathname.replace(/\/[^/]*$/, "/");
  return `${origin}${base}#/watch/${roomId}`;
}

// Status pill shown on each card. Processing shows a live percentage.
function StatusBadge({ status, progress }) {
  const styles = {
    ready: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
    processing: "bg-amber-500/15 text-amber-400 border-amber-500/30",
    failed: "bg-red-500/15 text-red-400 border-red-500/30",
    imported: "bg-white/10 text-muted border-white/15",
  };

  const label =
    status === "processing" && progress
      ? `${Math.round(progress.percent)}%`
      : status;

  return (
    <span
      className={`rounded-full border px-2 py-0.5 text-[11px] font-medium capitalize ${
        styles[status] || styles.imported
      }`}
    >
      {label}
    </span>
  );
}

export default function MediaLibrary() {
  const [items, setItems] = useState([]);
  const [player, setPlayer] = useState(null); // { media, session }
  // The queue is the host's own playlist, and the only view of the library that
  // viewers are ever told about. `queueIndex` is the entry playing right now, or
  // -1 when what is on screen did not come from the queue.
  const [queue, setQueue] = useState([]);
  const [queueIndex, setQueueIndex] = useState(-1);
  const [share, setShare] = useState(null); // { roomId, viewers, copied }
  const [shareError, setShareError] = useState("");
  const [uploadPct, setUploadPct] = useState(null);
  const [error, setError] = useState("");

  const progress = useProgress();
  const fileInputRef = useRef(null);

  // The live host session, and the <video> element being captured for it.
  const hostRef = useRef(null);
  const videoElRef = useRef(null);

  // Track (media, stage) pairs already handled so each triggers exactly one
  // refresh.
  const handledRef = useRef(new Set());

  // Event handlers read the queue through this ref rather than through state, so
  // they can never act on a stale queue.
  const queueRef = useRef({ items: [], index: -1 });

  const refresh = useCallback(async () => {
    try {
      setItems(await listMedia());
    } catch (err) {
      setError(err.message);
    }
  }, []);

  // Initial load.
  useEffect(() => {
    let ignore = false;
    (async () => {
      try {
        const data = await listMedia();
        if (!ignore) setItems(data);
      } catch (err) {
        if (!ignore) setError(err.message);
      }
    })();
    return () => {
      ignore = true;
    };
  }, []);

  // Refresh whenever something the UI shows changes server-side: a job reaching
  // a terminal state, or a thumbnail becoming available. Events are keyed by
  // `${id}:${stage}`, so each distinct stage triggers exactly one refresh even
  // though several events arrive per item. State is only updated after an await,
  // keeping it out of the synchronous render path.
  useEffect(() => {
    const REFRESH_STAGES = ["done", "failed", "thumbnail"];
    const pending = Object.values(progress).filter(
      (p) =>
        REFRESH_STAGES.includes(p.stage) &&
        !handledRef.current.has(`${p.media_id}:${p.stage}`),
    );
    if (pending.length === 0) return;

    pending.forEach((p) => handledRef.current.add(`${p.media_id}:${p.stage}`));

    let ignore = false;
    (async () => {
      try {
        const data = await listMedia();
        if (!ignore) setItems(data);
      } catch (err) {
        if (!ignore) setError(err.message);
      }
    })();
    return () => {
      ignore = true;
    };
  }, [progress]);

  const onFileChange = async (e) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setError("");
    setUploadPct(0);
    try {
      await uploadMedia(file, setUploadPct);
      await refresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setUploadPct(null);
      e.target.value = "";
    }
  };

  const onReprocess = async (id) => {
    setError("");
    ["done", "failed", "thumbnail"].forEach((stage) =>
      handledRef.current.delete(`${id}:${stage}`),
    );
    try {
      await reprocess(id);
      await refresh();
    } catch (err) {
      setError(err.message);
    }
  };

  const onDelete = async (id) => {
    setError("");
    try {
      await deleteMedia(id);
      // If the deleted item is open in the player, close it; the session itself
      // is released by the effect that owns it.
      if (player?.media.id === id) setPlayer(null);
      // A deleted item cannot stay in the queue either: the files are gone, and
      // viewers are being told exactly what is queued.
      removeFromQueue(id);
      await refresh();
    } catch (err) {
      setError(err.message);
    }
  };

  const stopSharing = () => {
    hostRef.current?.destroy();
    hostRef.current = null;
    videoElRef.current = null;
    setShareError("");
    setShare(null);
  };

  /**
   * Replace the queue and tell every viewer. The host owns the queue outright:
   * viewers are read-only and cannot ask the API for anything, so the whole
   * shared state travels over the data channel. `index` is the queue entry now
   * playing, or -1 when what is on screen did not come from the queue.
   */
  const publishQueue = (items, index) => {
    // The ref is written in the same tick as the state, so a handler that runs
    // before React has re-rendered (two clicks, an `ended` right after a change)
    // still sees the current queue.
    queueRef.current = { items, index };
    setQueue(items);
    setQueueIndex(index);
    hostRef.current?.setSnapshot(stateMessage({ items, index }));
  };

  /** Add an item to the end of the queue. */
  const addToQueue = (item) => {
    const { items, index } = queueRef.current;
    if (items.some((i) => i.id === item.id)) return;
    publishQueue([...items, item], index);
  };

  /**
   * Remove a queued item. Removing the one that is playing stops the queue
   * tracking it (index -1) rather than jumping to a neighbour: what is on screen
   * stays, but the queue will not auto-advance from a position that no longer
   * exists.
   */
  const removeFromQueue = (id) => {
    const { items, index } = queueRef.current;
    const at = items.findIndex((i) => i.id === id);
    if (at === -1) return;

    let nextIndex = index;
    if (at === index) nextIndex = -1;
    else if (at < index) nextIndex = index - 1;

    publishQueue(
      items.filter((i) => i.id !== id),
      nextIndex,
    );
  };

  const toggleQueue = (item) => {
    if (queueRef.current.items.some((i) => i.id === item.id)) {
      removeFromQueue(item.id);
      return;
    }
    addToQueue(item);
  };

  const clearQueue = () => publishQueue([], -1);

  /**
   * Advance when an item ends. Viewers need no equivalent: they are watching a
   * live stream of the host's player, so they follow automatically.
   */
  const handleEnded = () => {
    const { items, index } = queueRef.current;
    if (index < 0 || index + 1 >= items.length) return;
    playItem(items[index + 1]);
  };

  /**
   * Start playing an item — from the grid, from the queue, or by auto-advance.
   * A single entry point keeps `queueIndex` honest: an item is "at" its queue
   * position, or -1 when it is not in the queue at all.
   */
  const playItem = (item) => {
    const { items } = queueRef.current;
    const at = items.findIndex((i) => i.id === item.id);

    const session = new LocalSession({ src: playUrl(item.id) });
    // Subscribing here rather than in an effect keeps the handler out of a
    // dependency array; it reads refs only, so it cannot go stale.
    session.on("ended", handleEnded);

    setPlayer({ media: item, session });
    publishQueue(items, at);
  };

  const closePlayer = () => {
    stopSharing();
    // Dropping the player releases its session — see the effect below.
    setPlayer(null);
  };

  /**
   * Start sharing: open the player and create a room. The media stream is
   * captured from the host's own <video> element once it is mounted.
   */
  const startSharing = async (item) => {
    setError("");
    setShareError("");
    playItem(item);

    try {
      const host = new HostSession({
        onViewersChange: (viewers) =>
          setShare((s) => (s ? { ...s, viewers } : s)),
        // Surfaced in the player footer: a message on the page behind the modal
        // would never be seen.
        onError: (err) => setShareError(err.message),
      });
      hostRef.current = host;

      const roomId = await host.start();
      setShare({ roomId, viewers: 0, copied: false });

      // Tell viewers where the queue stands now, and keep it for whoever joins
      // later: the queue is the only view of the host they ever get.
      host.setSnapshot(stateMessage(queueRef.current));

      // The player may already be mounted; if so, capture from it now.
      if (videoElRef.current) handleVideoReady(videoElRef.current);
    } catch (err) {
      setShareError("Could not start sharing: " + err.message);
    }
  };

  /**
   * Called by the Player once it has attached the session, which is where the
   * host's <video> element becomes available. captureStream() forwards it as a
   * live MediaStream, so every viewer shares the host's clock automatically.
   */
  const handleVideoReady = (el) => {
    videoElRef.current = el;
    if (!el) return;

    const publish = () => {
      const host = hostRef.current;
      // Ignore a late load event from an element that has already been replaced
      // by the next queue item: publishing it would freeze viewers on the old
      // capture.
      if (!host || videoElRef.current !== el) return;
      if (el.captureStream) host.publishStream(el.captureStream());
    };

    // captureStream() before the element has frames produces a stream with no
    // tracks, which fails silently. Wait for the first frame if necessary.
    if (el.readyState >= 2) publish();
    else el.addEventListener("loadeddata", publish, { once: true });
  };

  const copyShareLink = async () => {
    if (!share) return;
    try {
      await navigator.clipboard.writeText(buildShareUrl(share.roomId));
      setShare((s) => (s ? { ...s, copied: true } : s));
      setTimeout(
        () => setShare((s) => (s ? { ...s, copied: false } : s)),
        2000,
      );
    } catch {
      setError("Could not copy the link — please copy it manually.");
    }
  };

  // Tear down any live session when the library unmounts.
  useEffect(
    () => () => {
      hostRef.current?.destroy();
    },
    [],
  );

  // The caller owns the session: Player attaches it but never destroys it. So
  // moving to the next queue item, or closing the player, has to release the one
  // that is being replaced.
  useEffect(() => {
    const session = player?.session;
    if (!session) return;
    return () => session.destroy();
  }, [player?.session]);

  return (
    <div className="min-h-full">
      {/* Header */}
      <header className="sticky top-0 z-30 border-b border-border bg-canvas/80 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
          <div className="flex items-center gap-2">
            <span className="grid h-8 w-8 place-items-center rounded-lg bg-accent text-sm font-bold text-white">
              L
            </span>
            <h1 className="text-base font-semibold tracking-tight">
              Local Media
            </h1>
          </div>

          <div className="flex items-center gap-3">
            {uploadPct !== null && (
              <span className="hidden text-xs text-muted sm:block">
                Uploading… {Math.round(uploadPct)}%
              </span>
            )}
            <input
              ref={fileInputRef}
              type="file"
              accept="video/*"
              onChange={onFileChange}
              className="hidden"
            />
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition hover:bg-accent-hover"
            >
              Upload video
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-6 sm:px-6">
        {error && (
          <p className="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">
            {error}
          </p>
        )}

        {queue.length > 0 && (
          <QueueList
            items={queue}
            index={queueIndex}
            onPlay={playItem}
            onRemove={removeFromQueue}
            onClear={clearQueue}
          />
        )}

        {items.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-border py-24 text-center">
            <p className="mb-1 text-sm font-medium">No media yet</p>
            <p className="mb-5 text-sm text-muted">
              Upload a video to get started. Playable files are ready instantly;
              others are remuxed or transcoded automatically.
            </p>
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition hover:bg-accent-hover"
            >
              Upload video
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4">
            {items.map((item) => {
              const p = progress[`${item.id}:processing`];
              const queuePosition = queue.findIndex((q) => q.id === item.id);
              return (
                <div
                  key={item.id}
                  className="group overflow-hidden rounded-xl border border-border bg-surface transition hover:border-accent/50"
                >
                  <div className="relative">
                    <button
                      type="button"
                      onClick={() => playItem(item)}
                      className="flex aspect-video w-full items-center justify-center bg-black"
                    >
                      {item.thumbnail ? (
                        <img
                          src={thumbnailUrl(item.thumbnail)}
                          alt={item.filename}
                          className="h-full w-full object-cover"
                        />
                      ) : (
                        <span className="text-xs text-muted">
                          {item.status === "processing"
                            ? "Processing…"
                            : "No preview"}
                        </span>
                      )}
                      <span className="absolute inset-0 grid place-items-center bg-black/0 opacity-0 transition group-hover:bg-black/30 group-hover:opacity-100">
                        <span className="grid h-12 w-12 place-items-center rounded-full bg-white/90 text-canvas">
                          <svg
                            viewBox="0 0 24 24"
                            fill="currentColor"
                            className="ml-0.5 h-6 w-6"
                          >
                            <path d="M8 5v14l11-7z" />
                          </svg>
                        </span>
                      </span>
                    </button>

                    {/* Queue toggle. A sibling of the play button rather than a
                        child of it: a button cannot nest inside a button. Only
                        ready items are queued — viewers are shown the order, so
                        it must only ever contain playable files. */}
                    {item.status === "ready" && (
                      <button
                        type="button"
                        onClick={() => toggleQueue(item)}
                        title={
                          queuePosition === -1
                            ? "Add to the queue"
                            : "Remove from the queue"
                        }
                        className={`absolute right-2 top-2 rounded-full border px-2 py-0.5 text-[11px] font-medium transition focus-visible:opacity-100 ${
                          queuePosition === -1
                            ? "border-white/20 bg-black/60 text-white/80 opacity-0 group-hover:opacity-100"
                            : "border-accent bg-accent text-white"
                        }`}
                      >
                        {queuePosition === -1
                          ? "Queue"
                          : `#${queuePosition + 1}`}
                      </button>
                    )}
                  </div>

                  <div className="p-3">
                    <p
                      className="truncate text-sm font-medium"
                      title={item.filename}
                    >
                      {item.filename}
                    </p>

                    <div className="mt-2 flex items-center justify-between gap-2">
                      <StatusBadge status={item.status} progress={p} />
                      <div className="flex items-center gap-1">
                        <button
                          type="button"
                          onClick={() => startSharing(item)}
                          disabled={item.status !== "ready"}
                          className="rounded-md px-2 py-1 text-[11px] font-medium text-accent transition hover:bg-accent/15 disabled:cursor-not-allowed disabled:text-muted disabled:hover:bg-transparent"
                        >
                          Share
                        </button>
                        <button
                          type="button"
                          onClick={() => onReprocess(item.id)}
                          className="rounded-md px-2 py-1 text-[11px] text-muted transition hover:bg-white/10 hover:text-foreground"
                        >
                          Re-process
                        </button>
                        <button
                          type="button"
                          onClick={() => onDelete(item.id)}
                          className="rounded-md px-2 py-1 text-[11px] text-muted transition hover:bg-red-500/15 hover:text-red-400"
                        >
                          Delete
                        </button>
                      </div>
                    </div>

                    {p?.stage === "processing" && (
                      <div className="mt-2 h-1 w-full overflow-hidden rounded-full bg-white/10">
                        <div
                          className="h-full rounded-full bg-accent transition-[width]"
                          style={{ width: `${p.percent}%` }}
                        />
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </main>

      {player && (
        <Player
          key={player.media.id}
          session={player.session}
          title={player.media.filename}
          subtitle={`${player.media.width}×${player.media.height} · ${player.media.video_codec} / ${player.media.audio_codec || "—"}`}
          onClose={closePlayer}
          onVideoReady={handleVideoReady}
          toolbar={
            <div className="flex min-w-0 items-center gap-2">
              {/* The queue lives in the player's chrome because the player is a
                  full-viewport modal: anything outside it is painted
                  underneath. */}
              <QueueBar
                items={queue}
                index={queueIndex}
                onNext={(nextIndex) => playItem(queue[nextIndex])}
              />

              {share ? (
                <ShareToolbar
                  url={buildShareUrl(share.roomId)}
                  viewers={share.viewers}
                  copied={share.copied}
                  onCopy={copyShareLink}
                  onStop={stopSharing}
                />
              ) : shareError ? (
                <span
                  className="max-w-[24rem] shrink-0 truncate text-xs text-red-400"
                  title={shareError}
                >
                  {shareError}
                </span>
              ) : null}
            </div>
          }
        />
      )}
    </div>
  );
}