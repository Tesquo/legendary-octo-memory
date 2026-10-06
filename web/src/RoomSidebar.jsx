import LibraryList from "./LibraryList";
import QueueList from "./QueueList";
import SharePanel from "./SharePanel";

/**
 * RoomSidebar is the persistent panel beside the theatre: the share link, who is
 * watching, and the playlist.
 *
 * The host also gets a way to grow the playlist — from a new upload or from a
 * file already in the library. A viewer gets the same playlist read-only, plus a
 * line explaining the connection, because everything it knows is pushed to it
 * over the data channel.
 */
export default function RoomSidebar({
  role = "host",
  share = null,
  shareError = "",
  onCopy,
  onStop,
  items = [],
  index = -1,
  onPlay,
  onRemove,
  onClear,
  library = [],
  onAdd,
  onUpload,
  uploadPct = null,
  statusText = "",
}) {
  const isHost = role === "host";

  return (
    <aside className="flex w-full flex-col gap-3 lg:w-80 lg:shrink-0">
      <div className="flex items-center gap-2">
        <span className="grid h-8 w-8 place-items-center rounded-lg bg-accent text-sm font-bold text-white">
          L
        </span>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold leading-tight">
            Watch party
          </p>
          <p className="text-[11px] text-muted">
            {isHost ? "You're hosting" : "You're watching"}
          </p>
        </div>
      </div>

      {isHost && share && (
        <SharePanel
          url={share.url}
          viewers={share.viewers}
          copied={share.copied}
          onCopy={onCopy}
          onStop={onStop}
        />
      )}

      {isHost && shareError && (
        <p className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400">
          {shareError}
        </p>
      )}

      {!isHost && statusText && (
        <p className="rounded-lg border border-border bg-surface-2 px-3 py-2 text-xs text-muted">
          {statusText}
        </p>
      )}

      <section className="rounded-2xl border border-border bg-surface p-3">
        <QueueList
          items={items}
          index={index}
          onPlay={onPlay}
          onRemove={onRemove}
          onClear={onClear}
          readOnly={!isHost}
        />
      </section>

      {isHost && (
        <section className="rounded-2xl border border-border bg-surface p-3">
          <div className="flex items-center justify-between gap-2">
            <h2 className="text-xs font-semibold uppercase tracking-wide text-muted">
              Add media
            </h2>
            <button
              type="button"
              onClick={onUpload}
              disabled={uploadPct !== null}
              className="rounded-md bg-accent px-2 py-1 text-[11px] font-medium text-white transition hover:bg-accent-hover disabled:opacity-60"
            >
              {uploadPct !== null
                ? `Uploading ${Math.round(uploadPct)}%`
                : "Upload"}
            </button>
          </div>

          <div className="mt-3 max-h-[24vh] overflow-y-auto pr-1">
            <LibraryList
              items={library}
              onSelect={onAdd}
              selectLabel="Add to the playlist"
              emptyText="Nothing else to add yet."
            />
          </div>
        </section>
      )}
    </aside>
  );
}
