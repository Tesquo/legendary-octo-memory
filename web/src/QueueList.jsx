/**
 * QueueList is the host's queue as a list: the one place queued items are shown
 * in order. It is host-only UI — viewers are read-only and are told about the
 * queue over the data channel instead (see lib/protocol.js).
 */
export default function QueueList({ items, index, onPlay, onRemove, onClear }) {
  if (items.length === 0) return null;

  return (
    <section className="mb-6 rounded-2xl border border-border bg-surface p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium">
          Queue{" "}
          <span className="text-muted">
            ({items.length} {items.length === 1 ? "item" : "items"})
          </span>
        </h2>

        <button
          type="button"
          onClick={onClear}
          className="rounded-md px-2 py-1 text-[11px] text-muted transition hover:bg-white/10 hover:text-foreground"
        >
          Clear
        </button>
      </div>

      <ol className="space-y-1">
        {items.map((item, i) => (
          <li
            key={item.id}
            className={`flex items-center gap-3 rounded-lg px-2 py-1.5 ${
              i === index ? "bg-accent/15" : "hover:bg-white/5"
            }`}
          >
            <span className="w-5 shrink-0 text-right text-[11px] tabular-nums text-muted">
              {i + 1}
            </span>

            <button
              type="button"
              onClick={() => onPlay(item)}
              title={item.filename}
              className="min-w-0 flex-1 truncate text-left text-sm"
            >
              {item.filename}
            </button>

            {i === index && (
              <span className="shrink-0 text-[11px] text-accent">Playing</span>
            )}

            <button
              type="button"
              onClick={() => onRemove(item.id)}
              aria-label={`Remove ${item.filename} from the queue`}
              className="shrink-0 rounded-md px-2 py-1 text-[11px] text-muted transition hover:bg-red-500/15 hover:text-red-400"
            >
              Remove
            </button>
          </li>
        ))}
      </ol>

      <p className="mt-3 text-xs text-muted">
        Only queued items are shared, and only in this order. Viewers cannot add
        to the queue, and never see the rest of your library.
      </p>
    </section>
  );
}
