import { useEffect, useState } from "react";
import { API_BASE } from "./api";

/**
 * Subscribes to the backend's Server-Sent Events progress stream.
 *
 * Events are keyed by `${media_id}:${stage}` rather than by media id alone.
 * That matters because a single media item emits several distinct stages
 * (queued, processing, thumbnail, done); keying only by id would let a later
 * event overwrite an earlier one and cause a refresh to be missed. The map
 * therefore retains the latest event per (media, stage) pair.
 *
 * Returns:
 *   { progress } where progress[`${id}:${stage}`] = event
 */
export function useProgress() {
  const [progress, setProgress] = useState({});

  useEffect(() => {
    const source = new EventSource(`${API_BASE}/progress`);

    source.onmessage = (e) => {
      try {
        const ev = JSON.parse(e.data);
        setProgress((prev) => ({ ...prev, [`${ev.media_id}:${ev.stage}`]: ev }));
      } catch {
        // Ignore malformed frames (e.g. keep-alive comments).
      }
    };

    return () => source.close();
  }, []);

  return progress;
}