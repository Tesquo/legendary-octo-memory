/**
 * ShareToolbar renders the live-sharing controls in the player's footer.
 *
 * It deliberately lives *inside* the player rather than as a floating bar: the
 * player is a full-viewport modal, so anything outside it is painted underneath
 * and invisible.
 */
export default function ShareToolbar({ url, viewers, copied, onCopy, onStop }) {
  return (
    <div className="flex shrink-0 items-center gap-2">
      <span className="hidden items-center gap-1.5 text-[11px] text-muted sm:flex">
        <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400" />
        {viewers} {viewers === 1 ? "viewer" : "viewers"}
      </span>

      <span className="hidden max-w-[16rem] truncate text-[11px] text-muted lg:block">
        {url}
      </span>

      <button
        type="button"
        onClick={onCopy}
        className="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-white transition hover:bg-accent-hover"
      >
        {copied ? "Copied" : "Copy link"}
      </button>

      <button
        type="button"
        onClick={onStop}
        className="rounded-lg px-3 py-1.5 text-xs text-muted transition hover:bg-white/10 hover:text-foreground"
      >
        Stop
      </button>
    </div>
  );
}
