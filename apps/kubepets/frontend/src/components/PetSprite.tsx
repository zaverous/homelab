import { useEffect, useState, type ComponentType, type SVGProps } from "react";
import {
  CreatureAngry,
  CreatureEatMouthClosed,
  CreatureEatMouthOpen,
  CreatureNormal,
  CreatureNormalEyesClosed,
  CreatureNormalHalfBlink,
  CreatureSad,
  CreatureSmile01,
  CreatureSmile02,
  CreatureSmile03,
  CreatureWith2Mouth,
  CreatureWith4Eyes,
} from "./icons/generated";

// Frame-swapping state machine over the hand-drawn creature assets.
// Mood degrades with hunger; each mood animates from real drawings:
//   idle      - drifts between the three smiles, blinks occasionally
//   uneasy    - neutral face, still blinks
//   gaunt     - the sad face, unblinking. it has stopped pretending.
//   desperate - MUTATES: cycles angry -> four eyes -> two mouths, trembling
//   feeding   - chomp loop (mouth open/closed) while the feed is in flight

export type Mood = "idle" | "uneasy" | "gaunt" | "desperate";

export function moodFor(hunger: number): Mood {
  if (hunger < 25) return "idle";
  if (hunger < 50) return "uneasy";
  if (hunger < 75) return "gaunt";
  return "desperate";
}

type Frame = ComponentType<SVGProps<SVGSVGElement>>;

const IDLE_FACES: Frame[] = [CreatureSmile01, CreatureSmile02, CreatureSmile03];
const HORRORS: Frame[] = [CreatureAngry, CreatureWith4Eyes, CreatureWith2Mouth];

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
  // 0 = eyes open, 1 = half-blink, 2 = closed
  const [blink, setBlink] = useState<0 | 1 | 2>(0);
  // shared frame counter for the chomp / mutation / smile-drift cycles
  const [cycle, setCycle] = useState(0);

  // chomp while the feed request is in flight
  useEffect(() => {
    if (!feeding) return;
    const t = setInterval(() => setCycle((c) => c + 1), 220);
    return () => clearInterval(t);
  }, [feeding]);

  // desperate: mutate between the three horror drawings
  useEffect(() => {
    if (feeding || mood !== "desperate") return;
    const t = setInterval(() => setCycle((c) => c + 1), 1600);
    return () => clearInterval(t);
  }, [mood, feeding]);

  // idle: drift lazily between the smile variants
  useEffect(() => {
    if (feeding || mood !== "idle") return;
    const t = setInterval(() => setCycle((c) => c + 1), 7000);
    return () => clearInterval(t);
  }, [mood, feeding]);

  // idle/uneasy: blink at random intervals (half -> closed -> half -> open)
  useEffect(() => {
    if (feeding || (mood !== "idle" && mood !== "uneasy")) {
      setBlink(0);
      return;
    }
    let cancelled = false;
    const timers: number[] = [];
    const schedule = () => {
      timers.push(
        window.setTimeout(() => {
          if (cancelled) return;
          setBlink(1);
          timers.push(window.setTimeout(() => setBlink(2), 110));
          timers.push(window.setTimeout(() => setBlink(1), 260));
          timers.push(
            window.setTimeout(() => {
              setBlink(0);
              schedule();
            }, 370),
          );
        }, 3500 + Math.random() * 3500),
      );
    };
    schedule();
    return () => {
      cancelled = true;
      timers.forEach(clearTimeout);
    };
  }, [mood, feeding]);

  let Face: Frame;
  if (feeding) {
    Face = cycle % 2 ? CreatureEatMouthClosed : CreatureEatMouthOpen;
  } else if (mood === "desperate") {
    Face = HORRORS[cycle % HORRORS.length];
  } else if (mood === "gaunt") {
    Face = CreatureSad;
  } else if (blink !== 0) {
    Face = blink === 2 ? CreatureNormalEyesClosed : CreatureNormalHalfBlink;
  } else {
    Face = mood === "idle" ? IDLE_FACES[cycle % IDLE_FACES.length] : CreatureNormal;
  }

  return (
    <Face
      className={`h-32 w-32 ${feeding ? "text-zinc-200" : tint[mood]} ${
        mood === "desperate" && !feeding ? "tremble" : ""
      }`}
    />
  );
}
