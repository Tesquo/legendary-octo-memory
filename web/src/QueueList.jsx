/**
 * QueueList is the room's playlist: the one place queued items are shown in
 * order, and the only view of the host's media that a viewer ever gets.
 *
 * It renders in the room sidebar for both roles. A host can play, remove and
 * clear entries; a viewer is read-only and cannot change anything — it is only
 * told about the queue over the data channel (see lib/protocol.js).
 */
export default function QueueList({
  items = [],
  index = -1,
  onPlay,
  onRemove,
  onClear,
  readOnly = false,
}) {
  return (
    <div>
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-muted">
          {readOnly ? "Up next" : "Playlist"}
        </h2>
        {!readOnly && items.length > 0 && (
          <button
            type="button"
            onClick={onClear}
            className="rounded-md px-2 py-0.5 text-[11px] text-muted transition hover:bg-white/10 hover:text-foreground"
          >
            Clear
          </button>
        )}
      </div>

      {items.length === 0 ? (
        <p className="mt-3 text-xs text-muted">
          {readOnly
            ? "Nothing queued yet."
            : "Nothing here yet — add media below."}
        </p>
      ) : (
        <ol className="mt-2 max-h-[34vh] space-y-1 overflow-y-auto pr-1">
          {items.map((item, i) => {
            const current = i === index;
            return (
              <li
                key={item.id}
                className={`group flex items-center gap-2 rounded-lg px-2 py-1.5 ${
                  current ? "bg-accent/15" : "hover:bg-white/5"
                }`}
              >
                <span className="w-4 shrink-0 text-right text-[11px] tabular-nums text-muted">
                  {i + 1}
                </span>

                {readOnly ? (
                  <span
                    title={item.filename}
                    className={`min-w-0 flex-1 truncate text-sm ${
                      current ? "" : "text-muted"
                    }`}
                  >
                    {item.filename}
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() => onPlay(item)}
                    title={item.filename}
                    className="min-w-0 flex-1 truncate text-left text-sm transition hover:text-accent"
                  >
                    {item.filename}
                  </button>
                )}

                {current && (
                  <span className="shrink-0 text-[10px] font-semibold uppercase tracking-wide text-accent">
                    On air
                  </span>
                )}

                {!readOnly && (
                  <button
                    type="button"
                    onClick={() => onRemove(item.id)}
                    aria-label={`Remove ${item.filename} from the playlist`}
                    className="shrink-0 rounded-md px-1 text-lg leading-none text-muted opacity-0 transition group-hover:opacity-100 hover:text-red-400 focus-visible:opacity-100"
                  >
                    ×
                  </button>
                )}
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}
