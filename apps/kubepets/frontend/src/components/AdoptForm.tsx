import { useState, type FormEvent } from "react";

export default function AdoptForm({ onAdopt }: { onAdopt: (name: string) => Promise<void> }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    try { await onAdopt(trimmed); setName(""); } finally { setBusy(false); }
  };
  return <form onSubmit={submit} className="adopt-form"><input value={name} onChange={(event) => setName(event.target.value)} placeholder="name the thing..." aria-label="Pet name" maxLength={40} /><button type="submit" disabled={busy || !name.trim()}>{busy ? "..." : "wake it"}</button></form>;
}
