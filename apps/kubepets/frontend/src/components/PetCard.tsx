import { useState } from "react";
import type { Pet } from "../api";
import HungerScratch from "./HungerScratch";
import PetSprite, { moodFor } from "./PetSprite";

const moodLabel = { idle: "content.", uneasy: "uneasy...", gaunt: "wasting away", desperate: "IT HUNGERS" } as const;

function relTime(iso?: string): string {
  if (!iso) return "never";
  const min = Math.floor((Date.now() - new Date(iso).getTime()) / 60000);
  if (min < 1) return "just now";
  if (min < 60) return `${min}m ago`;
  const hours = Math.floor(min / 60);
  return hours < 24 ? `${hours}h ago` : `${Math.floor(hours / 24)}d ago`;
}

function nameLengthClass(name: string): string {
  if (name.length > 20) return "pet-name-very-long";
  if (name.length > 12) return "pet-name-long";
  return "";
}

export default function PetCard({ pet, onFeed }: { pet: Pet; onFeed: (id: number) => Promise<void> }) {
  const [feeding, setFeeding] = useState(false);
  const mood = moodFor(pet.hunger);
  const critical = mood === "desperate";

  const feed = async () => {
    setFeeding(true);
    try { await Promise.all([onFeed(pet.id), new Promise((resolve) => setTimeout(resolve, 1200))]); }
    finally { setFeeding(false); }
  };

  return (
    <article className={`pet-stage mood-${mood}`}>
      <header className="pet-identity">
        <h2 className={nameLengthClass(pet.name)} title={pet.name}>{pet.name}</h2>
        <span>#{String(pet.id).padStart(3, "0")}</span>
      </header>
      <div className="pet-portrait">
        <div className="ritual-ring ritual-ring-outer" aria-hidden />
        <div className="ritual-ring ritual-ring-inner" aria-hidden />
        <PetSprite mood={mood} feeding={feeding} />
      </div>
      <p className="pet-mood">{moodLabel[mood]}</p>
      <div className="hunger-meter">
        <div className="hunger-label"><span>HUNGER</span><span>{pet.hunger} / 100</span></div>
        <HungerScratch value={pet.hunger} critical={critical} />
      </div>
      <button onClick={feed} disabled={feeding} className="feed-button"><span>{feeding ? "feeding..." : `feed ${pet.name}`}</span></button>
      <p className="last-fed">last fed {relTime(pet.last_fed_at)}</p>
    </article>
  );
}
