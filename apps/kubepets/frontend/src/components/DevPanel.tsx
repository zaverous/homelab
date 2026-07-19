import { useEffect, useState } from "react";
import { batchFeed, type ChaosResult } from "../api";

const CHAOS_COUNT = 5000;

// Dev Mode's chaos engine: one massive red button that floods the Redis queue
// via /chaos/batch-feed. Exists so the platform team can trigger worker
// autoscaling and resource-limit failures on demand from the UI.
export default function DevPanel({ onUnleashed, onClose }: { onUnleashed: () => void; onClose: () => void }) {
  const [busy, setBusy] = useState(false);
  const [armed, setArmed] = useState(false);
  const [result, setResult] = useState<ChaosResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) onClose();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [busy, onClose]);

  const unleash = async () => {
    if (busy) return;
    if (!armed) {
      setArmed(true);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      setResult(await batchFeed(CHAOS_COUNT));
      onUnleashed();
    } catch (e) {
      setResult(null);
      setError(e instanceof Error ? e.message : "the chaos refused to come");
    } finally {
      setBusy(false);
      setArmed(false);
    }
  };

  return (
    <aside className="dev-panel" aria-label="Dev mode chaos engine">
      <button className="dev-panel-close" onClick={onClose} disabled={busy} aria-label="Close stress test">×</button>
      <div className="chaos-sigil" aria-hidden>!</div>
      <h3>dev mode // chaos engine</h3>
      <p className="chaos-warning">the machinery beneath the enclosure</p>
      <button className="chaos-button" onClick={unleash} disabled={busy}>
        {busy ? "unleashing..." : armed ? "confirm: loose them" : `Trigger Chaos: ${CHAOS_COUNT.toLocaleString()} Synthetic Events`}
      </button>
      {armed && !busy && <button className="chaos-cancel" onClick={() => setArmed(false)}>keep the door shut</button>}
      {result && (
        <p className="chaos-result">
          {result.enqueued.toLocaleString()} events loosed. queue depth {result.queue_depth.toLocaleString()}.
          {result.backpressure && " redis is full - backpressure engaged."}
        </p>
      )}
      {error && <p className="chaos-result">{error}</p>}
    </aside>
  );
}
