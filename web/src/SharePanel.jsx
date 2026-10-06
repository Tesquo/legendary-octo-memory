import { useRef } from "react";

/**
 * SharePanel is the host's live-sharing control in the room sidebar: the link to
 * hand out, how many people are watching, and a way to end the session.
 *
 * The link is valid from the moment a room is opened — before the file is even
 * playable — so it can be sent out while preparation is still running.
 */
export default function SharePanel({ url, viewers, copied, onCopy, onStop }) {
  const inputRef = useRef(null);

  return (
    <section className="rounded-2xl border border-border bg-surface-2 p-3">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-muted">
          Invite
        </h2>
        <span className="flex items-center gap-1.5 text-[11px] text-muted">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400" />
          {viewers} watching
        </span>
      </div>

      <div className="mt-3 flex items-center gap-2">
        <input
          ref={inputRef}
          readOnly
          value={url}
          onFocus={() => inputRef.current?.select()}
          aria-label="Share link"
          className="min-w-0 flex-1 rounded-lg border border-border bg-canvas px-2 py-1.5 text-xs text-muted outline-none focus:border-accent"
        />
        <button
          type="button"
          onClick={onCopy}
          className="shrink-0 rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-white transition hover:bg-accent-hover"
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>

      <p className="mt-2 text-[11px] leading-snug text-muted">
        Anyone with this link can watch. Video goes straight from your browser to
        theirs.
      </p>

      <button
        type="button"
        onClick={onStop}
        className="mt-3 w-full rounded-lg border border-border px-3 py-1.5 text-xs text-muted transition hover:border-red-500/40 hover:bg-red-500/10 hover:text-red-400"
      >
        End session
      </button>
    </section>
  );
}
