import { useEffect, useState, type ComponentType, type SVGProps } from "react";
import {
  CreatureAngry,
  CreatureEatMouthClosed,
  CreatureEatMouthOpen,
  CreatureNormal,
  CreatureNormalEyesClosed,
  CreatureSad,
  CreatureSmile01,
  CreatureSmile02,
  CreatureWith2Mouth,
  CreatureWith4Eyes,
} from "./icons/generated";

// One canonical drawing per resting state. The expression is a PURE function of
// the pet's mood, so it changes exactly when hunger crosses a threshold - never
// on a timer:
//   idle      - content (a smile)
//   uneasy    - neutral
//   gaunt     - sad. it has stopped pretending.
//   desperate - angry/afraid (red + trembling via CSS, same drawing)
//   feeding   - chomp loop (mouth open/closed) while the feed is in flight
//
// ...but it REACTS TO BEING WATCHED: hovering the creature swaps to a second
// expression per mood - it notices you looking, and the closer you get to
// starvation the less it hides (extra eyes, extra mouths).

export type Mood = "idle" | "uneasy" | "gaunt" | "desperate";

export function moodFor(hunger: number): Mood {
  if (hunger < 25) return "idle";
  if (hunger < 50) return "uneasy";
  if (hunger < 75) return "gaunt";
  return "desperate";
}

type Frame = ComponentType<SVGProps<SVGSVGElement>>;

const FACE: Record<Mood, Frame> = {
  idle: CreatureSmile01,
  uneasy: CreatureNormal,
  gaunt: CreatureSad,
  desperate: CreatureAngry,
};

// The "it noticed you" face, shown while hovering (or focused).
const HOVER_FACE: Record<Mood, Frame> = {
  idle: CreatureSmile02, // perks up, a wider grin
  uneasy: CreatureNormalEyesClosed, // averts its eyes
  gaunt: CreatureWith4Eyes, // more eyes open to look back
  desperate: CreatureWith2Mouth, // it stops hiding the second mouth
};

const tint: Record<Mood, string> = {
  idle: "text-zinc-300",
  uneasy: "text-zinc-400",
  gaunt: "text-zinc-500",
  desperate: "text-red-400",
};

export default function PetSprite({
  mood,
  feeding = false,
}: {
  mood: Mood;
  feeding?: boolean;
}) {
  // The only per-frame animation left: the chomp while a feed is in flight.
  const [chomp, setChomp] = useState(false);
  useEffect(() => {
    if (!feeding) {
      setChomp(false);
      return;
    }
    const t = setInterval(() => setChomp((c) => !c), 200);
    return () => clearInterval(t);
  }, [feeding]);

  // hover/focus: it reacts to being watched (ignored mid-feed - it's busy)
  const [watched, setWatched] = useState(false);

  const Face: Frame = feeding
    ? chomp
      ? CreatureEatMouthClosed
      : CreatureEatMouthOpen
    : watched
      ? HOVER_FACE[mood]
      : FACE[mood];

  return (
    <div
      className={`pet-sprite-frame pet-sprite-${mood} ${feeding ? "pet-sprite-feeding" : ""} ${watched && !feeding ? "pet-sprite-watched" : ""}`}
      role="img"
      aria-label={`The creature is ${feeding ? "feeding" : mood}`}
      tabIndex={0}
      onMouseEnter={() => setWatched(true)}
      onMouseLeave={() => setWatched(false)}
      onFocus={() => setWatched(true)}
      onBlur={() => setWatched(false)}
    >
      <Face
        className={`pet-sprite-art ${feeding ? "text-zinc-200" : tint[mood]} ${
          mood === "desperate" && !feeding ? "tremble" : ""
        }`}
      />
    </div>
  );
}
