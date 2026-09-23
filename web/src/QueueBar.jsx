/**
 * QueueBar is the compact "where are we" control in the player's footer.
 *
 * It renders inside the player because the player is a full-viewport modal:
 * anything outside it is painted underneath and invisible (see SKILL.md). The
 * host gets a Next button; a viewer is read-only and just sees where the queue
 * stands, since it has no way to change it.
 */
export default function QueueBar({ items = [], index = -1, onNext }) {
  if (items.length === 0) return null;

  const nextIndex = index >= 0 ? index + 1 : 0;
  const upNext = items[nextIndex] || null;
  const remaining = items.length - nextIndex;

  return (
    <div className="flex shrink-0 items-center gap-2">
      <span className="hidden text-[11px] text-muted sm:block">
        {remaining} queued
      </span>

      <span
        className="hidden max-w-[18rem] truncate text-[11px] text-muted lg:block"
        title={upNext ? upNext.filename : undefined}
      >
        {upNext ? `Up next — ${upNext.filename}` : "End of queue"}
      </span>

      {onNext && upNext && (
        <button
          type="button"
          onClick={() => onNext(nextIndex)}
          className="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-white transition hover:bg-accent-hover"
        >
          Next
        </button>
      )}
    </div>
  );
}
