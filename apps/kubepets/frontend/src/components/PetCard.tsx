import { useState } from "react";
import type { Pet } from "../api";
import PetSprite, { moodFor } from "./PetSprite";

const moodLabel = {
  idle: "idle.",
  uneasy: "uneasy...",
  gaunt: "wasting away",
  desperate: "IT HUNGERS",
} as const;

function barColor(hunger: number): string {
  if (hunger < 25) return "bg-zinc-500";
  if (hunger < 50) return "bg-amber-700";
  if (hunger < 75) return "bg-orange-700";
  return "bg-red-600";
}

function relTime(iso?: string): string {
  if (!iso) return "never";
  const ms = Date.now() - new Date(iso).getTime();
  const min = Math.floor(ms / 60000);
  if (min < 1) return "just now";
  if (min < 60) return `${min}m ago`;
  const h = Math.floor(min / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

export default function PetCard({
  pet,
  onFeed,
}: {
  pet: Pet;
  onFeed: (id: number) => Promise<void>;
}) {
  const [feeding, setFeeding] = useState(false);
  const mood = moodFor(pet.hunger);
  const critical = mood === "desperate";

  const feed = async () => {
    setFeeding(true);
    try {
      // hold the chomp animation at least ~1.2s even if the API is instant -
      // the eat frames are the payoff for clicking
      await Promise.all([
        onFeed(pet.id),
        new Promise((r) => setTimeout(r, 1200)),
      ]);
    } finally {
      setFeeding(false);
    }
  };

  return (
    <div
      className={`relative z-10 flex flex-col items-center gap-3 border bg-zinc-900/95 p-5 ${
        critical ? "border-red-900/80 shadow-[0_0_24px_rgba(127,29,29,0.35)]" : "border-zinc-800"
      }`}
    >
      <div className="flex w-full items-baseline justify-between">
        <h2 className="font-mono text-sm uppercase tracking-[0.25em] text-zinc-300">
          {pet.name}
        </h2>
        <span className="font-mono text-[10px] text-zinc-600">#{pet.id}</span>
      </div>

      <PetSprite mood={mood} feeding={feeding} />

      <p
        className={`font-mono text-xs ${
          critical ? "throb text-red-500" : "text-zinc-500"
        }`}
      >
        {moodLabel[mood]}
      </p>

      {/* hunger bar - fills toward starvation */}
      <div className="w-full">
        <div className="mb-1 flex justify-between font-mono text-[10px] text-zinc-600">
          <span>hunger</span>
          <span className={critical ? "text-red-500" : ""}>{pet.hunger}/100</span>
        </div>
        <div className="h-2 w-full border border-zinc-800 bg-zinc-950">
          <div
            className={`h-full transition-all duration-700 ${barColor(pet.hunger)} ${critical ? "throb" : ""}`}
            style={{ width: `${pet.hunger}%` }}
          />
        </div>
      </div>

      <button
        onClick={feed}
        disabled={feeding}
        className={`w-full border py-2 font-mono text-xs uppercase tracking-[0.3em] transition-colors disabled:opacity-40 ${
          critical
            ? "border-red-800 bg-red-950/60 text-red-300 hover:bg-red-900/60"
            : "border-zinc-700 text-zinc-400 hover:border-zinc-500 hover:text-zinc-200"
        }`}
      >
        {feeding ? "feeding..." : "feed"}
      </button>

      <p className="font-mono text-[10px] text-zinc-700">
        last fed: {relTime(pet.last_fed_at)}
      </p>
    </div>
  );
}
