/**
 * Status pill for a media item. A job in flight shows its live percentage once
 * the progress stream has reported one.
 */
export function StatusBadge({ status, progress }) {
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
      className={`shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-medium capitalize ${
        styles[status] || styles.imported
      }`}
    >
      {label}
    </span>
  );
}

/**
 * LibraryList is the host's own files as compact rows — deliberately not a grid
 * of posters. It is used on the home screen (where a row opens a room) and in the
 * room sidebar (where a row joins the playlist).
 *
 * Only the host ever sees this. Viewers are told about the playlist over the data
 * channel and never learn about the rest of the library.
 */
export default function LibraryList({
  items = [],
  progress = {},
  onSelect,
  selectLabel = "Open",
  onDelete,
  onReprocess,
  emptyText = "No media yet.",
}) {
  if (items.length === 0) {
    return <p className="text-xs text-muted">{emptyText}</p>;
  }

  return (
    <ul className="space-y-1">
      {items.map((item) => (
        <li
          key={item.id}
          className="group flex items-center gap-2 rounded-lg border border-transparent px-2 py-1.5 transition hover:border-border hover:bg-white/5"
        >
          <button
            type="button"
            onClick={() => onSelect(item)}
            title={`${selectLabel}: ${item.filename}`}
            className="min-w-0 flex-1 truncate text-left text-sm transition hover:text-accent"
          >
            {item.filename}
          </button>

          <StatusBadge
            status={item.status}
            progress={progress[`${item.id}:processing`]}
          />

          {(onReprocess || onDelete) && (
            <span className="flex shrink-0 items-center gap-0.5 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100">
              {onReprocess && (
                <button
                  type="button"
                  onClick={() => onReprocess(item.id)}
                  className="rounded-md px-1.5 py-0.5 text-[11px] text-muted transition hover:bg-white/10 hover:text-foreground"
                >
                  Re-process
                </button>
              )}
              {onDelete && (
                <button
                  type="button"
                  onClick={() => onDelete(item.id)}
                  className="rounded-md px-1.5 py-0.5 text-[11px] text-muted transition hover:bg-red-500/15 hover:text-red-400"
                >
                  Delete
                </button>
              )}
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}
