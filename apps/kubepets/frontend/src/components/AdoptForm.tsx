import { useState, type FormEvent } from "react";

export default function AdoptForm({
  onAdopt,
}: {
  onAdopt: (name: string) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    try {
      await onAdopt(trimmed);
      setName("");
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="relative z-10 flex gap-2">
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="name the thing..."
        maxLength={40}
        className="w-56 border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-xs text-zinc-300 placeholder-zinc-700 outline-none focus:border-zinc-600"
      />
      <button
        type="submit"
        disabled={busy || !name.trim()}
        className="border border-zinc-700 px-4 py-2 font-mono text-xs uppercase tracking-[0.3em] text-zinc-400 transition-colors hover:border-zinc-500 hover:text-zinc-200 disabled:opacity-40"
      >
        {busy ? "..." : "adopt"}
      </button>
    </form>
  );
}
