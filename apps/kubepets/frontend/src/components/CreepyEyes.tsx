import { useEffect, useState, type ComponentType, type SVGProps } from "react";
import {
  EyeR1C1RingPupil,
  EyeR1C2VerticalSlit,
  EyeR2C1HalfOpenPupil,
  EyeR2C2HalfOpenShadow,
  EyeR3C1ClosedLongLashes,
  EyeR3C2ClosedShortLashes,
} from "./icons/generated";

// The chaos mechanic (4c spec): as hunger rises, distinct hand-drawn "creepy
// eye" components spawn at random absolute X/Y positions across the dark. The
// useEffect adds a batch whenever the target count rises and culls when it
// falls; existing eyes keep their position/variant (no reshuffle on rerender).

type EyeComponent = ComponentType<SVGProps<SVGSVGElement>>;

// First four are open eyes (they get the CSS blink); the closed ones just wait.
const VARIANTS: EyeComponent[] = [
  EyeR1C1RingPupil,
  EyeR1C2VerticalSlit,
  EyeR2C1HalfOpenPupil,
  EyeR2C2HalfOpenShadow,
  EyeR3C1ClosedLongLashes,
  EyeR3C2ClosedShortLashes,
];
const OPEN_VARIANTS = 4;

interface Eye {
  id: number;
  variant: number;
  x: number; // vw %
  y: number; // vh %
  size: number; // px
  rot: number; // deg
  delay: number; // s, staggers the blink so they don't sync
}

let nextId = 1;

function spawnEye(): Eye {
  return {
    id: nextId++,
    variant: Math.floor(Math.random() * VARIANTS.length),
    x: Math.random() * 92,
    y: Math.random() * 90,
    size: 44 + Math.random() * 72,
    rot: -20 + Math.random() * 40,
    delay: Math.random() * 5,
  };
}

export default function CreepyEyes({ intensity }: { intensity: number }) {
  // 0 eyes while everyone is fed; ~1 eye per 8 points of worst-case hunger,
  // capped at 12 so the DOM (and the vibe) stays under control.
  const target = Math.min(12, Math.floor(Math.max(0, intensity) / 8));
  const [eyes, setEyes] = useState<Eye[]>([]);

  useEffect(() => {
    setEyes((prev) => {
      if (prev.length === target) return prev;
      if (prev.length > target) return prev.slice(0, target);
      const fresh = Array.from({ length: target - prev.length }, spawnEye);
      return [...prev, ...fresh];
    });
  }, [target]);

  return (
    <div className="pointer-events-none fixed inset-0 z-0 overflow-hidden" aria-hidden>
      {eyes.map((e) => {
        const Variant = VARIANTS[e.variant];
        const blinks = e.variant < OPEN_VARIANTS;
        return (
          <div
            key={e.id}
            className="emerge absolute"
            style={{
              left: `${e.x}%`,
              top: `${e.y}%`,
              width: e.size,
              transform: `rotate(${e.rot}deg)`,
            }}
          >
            <Variant
              className={`h-auto w-full text-zinc-500 ${blinks ? "blink" : ""}`}
              style={blinks ? { animationDelay: `${e.delay}s` } : undefined}
            />
          </div>
        );
      })}
    </div>
  );
}
