import { useCallback, useEffect, useRef, useState } from "react";
import {
  listMedia,
  uploadMedia,
  reprocess,
  deleteMedia,
  playUrl,
} from "./lib/api";
import { LocalSession } from "./lib/MediaSession";
import { HostSession } from "./lib/webrtc";
import { stateMessage } from "./lib/protocol";
import { useProgress } from "./lib/useProgress";
import Player from "./Player";
import RoomSidebar from "./RoomSidebar";
import StagePanel from "./StagePanel";
import LibraryList from "./LibraryList";

/** Build the viewer link for a room, honouring any sub-path deployment. */
function buildShareUrl(roomId) {
  const { origin, pathname } = window.location;
  const base = pathname.replace(/\/[^/]*$/, "/");
  return `${origin}${base}#/watch/${roomId}`;
}

/**
 * MediaLibrary is the host's side of the app, and it is room-first: choosing a
 * file opens a room immediately — so the share link exists while the file is
 * still being prepared — and playback starts from a deliberate press.
 *
 * The host is the only role that talks to the Go API. Viewers get the playlist
 * over the data channel and the video over WebRTC (see WatchPage).
 */
export default function MediaLibrary() {
  const [items, setItems] = useState([]);

  // The media the room is centred on. Setting it opens the room; it is what is
  // being prepared, what is ready to start, or what is playing.
  const [focusedId, setFocusedId] = useState(null);
  const [player, setPlayer] = useState(null); // { media, session }

  // The playlist is the host's own queue, and the only view of the library that
  // viewers are ever told about. `queueIndex` is the entry on air, or -1.
  const [queue, setQueue] = useState([]);
  const [queueIndex, setQueueIndex] = useState(-1);

  const [share, setShare] = useState(null); // { roomId, viewers, copied }
  const [shareError, setShareError] = useState("");
  const [uploadPct, setUploadPct] = useState(null);
  const [error, setError] = useState("");
  const [dragActive, setDragActive] = useState(false);

  const progress = useProgress();
  const fileInputRef = useRef(null);

  // The live host session, and the <video> element being captured for it.
  const hostRef = useRef(null);
  const videoElRef = useRef(null);

  // Track (media, stage) pairs already handled so each triggers exactly one
  // refresh.
  const handledRef = useRef(new Set());

  // Handlers read the queue through this ref rather than through state, so they
  // can never act on a stale queue.
  const queueRef = useRef({ items: [], index: -1 });

  const focusedItem = items.find((i) => i.id === focusedId) || null;
  const inRoom = focusedId !== null;

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
  /**
   * Upload a file and centre the room on it. Choosing a file is what opens the
   * room, so its link exists from here on and can be shared while the file is
   * still being prepared. A file chosen while a room is already running simply
   * becomes what the room plays next.
   */
  const uploadFile = async (file) => {
    setError("");
    setUploadPct(0);
    try {
      const uploaded = await uploadMedia(file, setUploadPct);
      await refresh();
      openRoom(uploaded.id);
    } catch (err) {
      setError(err.message);
    } finally {
      setUploadPct(null);
    }
  };

  const onFileChange = (e) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (file) uploadFile(file);
  };

  const onDrop = (e) => {
    e.preventDefault();
    setDragActive(false);
    const file = e.dataTransfer?.files?.[0];
    if (file) uploadFile(file);
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
      if (focusedId === id) {
        // The room's media is gone, so end the session rather than leaving
        // viewers attached to a room with nothing in it.
        leaveRoom();
      } else {
        // A deleted item cannot stay in the playlist either: the files are gone,
        // and viewers are being told exactly what is queued.
        if (player?.media.id === id) setPlayer(null);
        removeFromQueue(id);
      }
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
   * Open a room centred on a media item, creating the peer session the first
   * time. The link is live from this point — before the file is playable — so it
   * can be handed out while preparation is still running.
   */
  const openRoom = async (id) => {
    setError("");
    setShareError("");
    setFocusedId(id);

    // Moving the room to a different item stops whatever is on screen, so the
    // stage can show the new file being prepared or ready to start.
    if (player && player.media.id !== id) setPlayer(null);

    if (hostRef.current) return;

    try {
      const host = new HostSession({
        onViewersChange: (viewers) =>
          setShare((s) => (s ? { ...s, viewers } : s)),
        onError: (err) => setShareError(err.message),
      });
      hostRef.current = host;

      const roomId = await host.start();
      // The queue is the only view of the host a viewer ever gets, so it is sent
      // as soon as there is a room to send it to.
      host.setSnapshot(stateMessage(queueRef.current));
      setShare({ roomId, viewers: 0, copied: false });

      // The player may already be mounted (a file added mid-session); capture
      // from it now rather than waiting for another load event.
      if (videoElRef.current) handleVideoReady(videoElRef.current);
    } catch (err) {
      setShareError("Could not start sharing: " + err.message);
    }
  };

  /** Leave the room: end the peer session and clear everything it owned. */
  const leaveRoom = () => {
    stopSharing();
    setPlayer(null);
    setFocusedId(null);
    publishQueue([], -1);
  };
  /**
   * Replace the playlist and tell every viewer. The host owns the queue
   * outright: viewers are read-only and cannot ask the API for anything, so the
   * whole shared state travels over the data channel. `index` is the queue entry
   * on air, or -1 when nothing on the playlist is playing.
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

  /** Add an item to the end of the playlist without playing it. */
  const addToQueue = (item) => {
    const { items, index } = queueRef.current;
    if (items.some((i) => i.id === item.id)) return;
    publishQueue([...items, item], index);
  };

  /**
   * Remove a playlist entry. Dropping the entry that is on air stops it too: the
   * playlist is the shared truth about what is playing, so a viewer is never
   * left watching something the queue does not name.
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

    if (at === index && player?.media.id === id) setPlayer(null);
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
   * Play an item — from the playlist, or when the room's file becomes ready. It
   * joins the playlist if it is not on it, so the queue always names what is on
   * air. A single entry point keeps `queueIndex` honest: an item is on air at
   * its playlist position.
   */
  const playItem = (item) => {
    const { items } = queueRef.current;
    let next = items;
    let at = items.findIndex((i) => i.id === item.id);
    if (at === -1) {
      next = [...items, item];
      at = next.length - 1;
    }

    const session = new LocalSession({ src: playUrl(item.id) });
    // Subscribing here rather than in an effect keeps the handler out of a
    // dependency array; it reads refs only, so it cannot go stale.
    session.on("ended", handleEnded);

    setFocusedId(item.id);
    setPlayer({ media: item, session });
    publishQueue(next, at);
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
  // being replaced.
  useEffect(() => {
    const session = player?.session;
    if (!session) return;
    return () => session.destroy();
  }, [player?.session]);

  /** Ready files that are not already on the playlist. */
  const queuedIds = new Set(queue.map((i) => i.id));
  const addableMedia = items.filter(
    (i) => i.status === "ready" && !queuedIds.has(i.id),
  );

  const fileInput = (
    <input
      ref={fileInputRef}
      type="file"
      accept="video/*"
      onChange={onFileChange}
      className="hidden"
    />
  );

  // --- The room: a theatre with its sidebar ---------------------------------
  if (inRoom) {
    return (
      <div
        className="min-h-screen bg-canvas"
        onDragOver={(e) => {
          e.preventDefault();
          setDragActive(true);
        }}
        onDragLeave={() => setDragActive(false)}
        onDrop={onDrop}
      >
        <header className="border-b border-border">
          <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
            <div className="flex items-center gap-2">
              <span className="grid h-8 w-8 place-items-center rounded-lg bg-accent text-sm font-bold text-white">
                L
              </span>
              <h1 className="text-base font-semibold tracking-tight">
                Watch together
              </h1>
            </div>
            <button
              type="button"
              onClick={leaveRoom}
              className="rounded-lg border border-border px-3 py-1.5 text-xs text-muted transition hover:bg-white/10 hover:text-foreground"
            >
              Leave room
            </button>
          </div>
        </header>

        <div className="mx-auto max-w-6xl px-4 sm:px-6">
          {error && (
            <p className="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">
              {error}
            </p>
          )}

          <div className="flex flex-col gap-4 py-4 lg:flex-row lg:items-start">
            <div className="flex min-w-0 justify-center lg:flex-1">
              {player ? (
                <Player
                  key={player.media.id}
                  mode="theatre"
                  session={player.session}
                  title={player.media.filename}
                  subtitle={`${player.media.width}×${player.media.height} · ${player.media.video_codec} / ${player.media.audio_codec || "—"}`}
                  onClose={leaveRoom}
                  onVideoReady={handleVideoReady}
                />
              ) : (
                /* No player yet: the room is open, and the stage shows the
                   file being prepared or waiting to start. */
                <StagePanel
                  item={focusedItem}
                  progress={
                    focusedItem
                      ? progress[`${focusedItem.id}:processing`]
                      : null
                  }
                  onStart={() => playItem(focusedItem)}
                />
              )}
            </div>

            <RoomSidebar
              role="host"
              share={
                share ? { ...share, url: buildShareUrl(share.roomId) } : null
              }
              shareError={shareError}
              onCopy={copyShareLink}
              onStop={leaveRoom}
              items={queue}
              index={queueIndex}
              onPlay={playItem}
              onRemove={removeFromQueue}
              onClear={clearQueue}
              library={addableMedia}
              onAdd={addToQueue}
              onUpload={() => fileInputRef.current?.click()}
              uploadPct={uploadPct}
            />
          </div>
        </div>

        {fileInput}

        {dragActive && (
          <p className="pointer-events-none fixed bottom-6 left-1/2 z-40 -translate-x-1/2 rounded-full border border-accent bg-surface px-4 py-2 text-xs shadow-lg">
            Drop the video to add it to the room
          </p>
        )}
      </div>
    );
  }
  // --- Home: choose something, or reopen something --------------------------
  return (
    <div
      className="min-h-full"
      onDragOver={(e) => {
        e.preventDefault();
        setDragActive(true);
      }}
      onDragLeave={() => setDragActive(false)}
      onDrop={onDrop}
    >
      <header className="sticky top-0 z-30 border-b border-border bg-canvas/80 backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center gap-2 px-4 py-3 sm:px-6">
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-accent text-sm font-bold text-white">
            L
          </span>
          <h1 className="text-base font-semibold tracking-tight">
            Watch together
          </h1>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-4 py-8 sm:px-6">
        {error && (
          <p className="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">
            {error}
          </p>
        )}

        {/* The one thing to do on this screen: choose something to watch. */}
        <button
          type="button"
          onClick={() => fileInputRef.current?.click()}
          disabled={uploadPct !== null}
          className={`flex w-full flex-col items-center justify-center rounded-3xl border border-dashed px-6 py-16 text-center transition disabled:opacity-60 ${
            dragActive
              ? "border-accent bg-accent/10"
              : "border-border bg-surface hover:border-accent/60 hover:bg-surface-2"
          }`}
        >
          <span className="grid h-14 w-14 place-items-center rounded-2xl bg-accent text-white">
            <svg
              viewBox="0 0 24 24"
              fill="currentColor"
              className="h-7 w-7"
              aria-hidden="true"
            >
              <path d="M8 5v14l11-7z" />
            </svg>
          </span>
          <span className="mt-5 text-lg font-semibold">
            {uploadPct !== null
              ? `Uploading… ${Math.round(uploadPct)}%`
              : "Select media to start a watch party"}
          </span>
          <span className="mt-2 max-w-md text-sm text-muted">
            Pick a video from this machine, or drop it here. A room opens
            straight away with a link you can send to anyone — and you keep
            control of playback.
          </span>
        </button>

        {fileInput}

        {items.length > 0 && (
          <section className="mt-10">
            <h2 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted">
              Or open something from your library
            </h2>
            <LibraryList
              items={items}
              progress={progress}
              onSelect={(item) => openRoom(item.id)}
              selectLabel="Open in a room"
              onReprocess={onReprocess}
              onDelete={onDelete}
            />
          </section>
        )}
      </main>
    </div>
  );
}




