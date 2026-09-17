import { useCallback, useEffect, useRef, useState } from "react";
import {
  listMedia,
  uploadMedia,
  reprocess,
  deleteMedia,
  thumbnailUrl,
} from "./lib/api";
import { useProgress } from "./lib/useProgress";
import Player from "./Player";

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
  const [selected, setSelected] = useState(null);
  const [uploadPct, setUploadPct] = useState(null);
  const [error, setError] = useState("");

  const progress = useProgress();
  const fileInputRef = useRef(null);

  // Track (media, stage) pairs already handled so each triggers exactly one
  // refresh.
  const handledRef = useRef(new Set());

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
      setSelected((current) => (current?.id === id ? null : current));
      await refresh();
    } catch (err) {
      setError(err.message);
    }
  };

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
              return (
                <div
                  key={item.id}
                  className="group overflow-hidden rounded-xl border border-border bg-surface transition hover:border-accent/50"
                >
                  <button
                    type="button"
                    onClick={() => setSelected(item)}
                    className="relative flex aspect-video w-full items-center justify-center bg-black"
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

      <Player media={selected} onClose={() => setSelected(null)} />
    </div>
  );
}