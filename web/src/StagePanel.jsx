import { thumbnailUrl } from "./lib/api";

/**
 * StagePanel fills the theatre while there is no player yet: the room's media as
 * it is being prepared (with live progress), or its poster and a start button
 * once it is ready.
 *
 * It exists so a room — and its share link — can be opened before the file is
 * playable. Playback still starts from a deliberate click, which is also what
 * gives the WebRTC capture a definite starting point.
 */
export default function StagePanel({ item, progress, onStart }) {
  if (!item) {
    return (
      <div className="flex aspect-video w-full min-w-0 flex-1 items-center justify-center rounded-2xl border border-border bg-surface shadow-lg">
        <span className="h-8 w-8 animate-spin rounded-full border-2 border-border border-t-accent" />
      </div>
    );
  }

  const isReady = item.status === "ready";
  const failed = item.status === "failed";
  const pct = progress?.percent ?? 0;

  return (
    <div className="flex w-full min-w-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border bg-surface shadow-lg">
      <div className="relative grid aspect-video w-full place-items-center bg-black">
        {item.thumbnail && (
          <img
            src={thumbnailUrl(item.thumbnail)}
            alt=""
            className={`absolute inset-0 h-full w-full object-cover ${
              isReady ? "opacity-50" : "opacity-25"
            }`}
          />
        )}

        {isReady ? (
          <button
            type="button"
            onClick={onStart}
            className="relative flex flex-col items-center gap-3"
          >
            <span className="grid h-16 w-16 place-items-center rounded-full bg-white/90 text-canvas shadow-lg transition hover:scale-105">
              <svg
                viewBox="0 0 24 24"
                fill="currentColor"
                className="h-7 w-7"
                aria-hidden="true"
              >
                <path d="M8 5v14l11-7z" />
              </svg>
            </span>
            <span className="text-sm font-medium text-white">
              Start watching
            </span>
          </button>
        ) : (
          <div className="relative w-full max-w-sm px-6 text-center">
            <p className="text-sm font-medium text-white">
              {failed
                ? "This file could not be prepared."
                : "Preparing your video…"}
            </p>

            {!failed && (
              <>
                <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-white/20">
                  <div
                    className="h-full rounded-full bg-accent transition-[width]"
                    style={{ width: `${pct}%` }}
                  />
                </div>
                <p className="mt-2 text-xs text-white/70">
                  {progress?.message || "Waiting for the encoder…"}
                  {pct > 0 ? ` · ${Math.round(pct)}%` : ""}
                </p>
              </>
            )}
          </div>
        )}
      </div>

      <div className="flex items-center justify-between gap-4 border-t border-border px-4 py-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium" title={item.filename}>
            {item.filename}
          </p>
          <p className="truncate text-[11px] text-muted">
            {item.width}×{item.height}
            {item.video_codec ? ` · ${item.video_codec}` : ""}
            {item.audio_codec ? ` / ${item.audio_codec}` : ""}
          </p>
        </div>
        <span className="shrink-0 rounded-full border border-border px-2 py-0.5 text-[11px] capitalize text-muted">
          {isReady ? "Ready" : failed ? "Failed" : "Preparing"}
        </span>
      </div>
    </div>
  );
}
