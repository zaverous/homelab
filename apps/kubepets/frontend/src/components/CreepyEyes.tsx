import { useCallback, useEffect, useRef, useState, type ComponentType, type SVGProps } from "react";
import {
  EyeR1C1RingPupil,
  EyeR1C2VerticalSlit,
  EyeR2C1HalfOpenPupil,
  EyeR2C2HalfOpenShadow,
  EyeR3C1ClosedLongLashes,
  EyeR3C2ClosedShortLashes,
} from "./icons/generated";

// The chaos mechanic: hand-drawn eyes that open and close all over the dark.
// Each eye fades in at a random spot, blinks on its own rhythm (swapping its
// open drawing to a closed one for a beat), lives a random lifetime, then fades
// out and removes itself - so eyes are constantly appearing and vanishing
// somewhere on screen rather than sitting in fixed slots. Population scales with
// the hungriest pet: calm -> a lone lurker, starving -> a crowd.

type EyeComponent = ComponentType<SVGProps<SVGSVGElement>>;

const OPEN: EyeComponent[] = [
  EyeR1C1RingPupil,
  EyeR1C2VerticalSlit,
  EyeR2C1HalfOpenPupil,
  EyeR2C2HalfOpenShadow,
];
const CLOSED: EyeComponent[] = [EyeR3C1ClosedLongLashes, EyeR3C2ClosedShortLashes];

interface EyeSpec {
  id: number;
  x: number; // vw %
  y: number; // vh %
  size: number; // px
  rot: number; // deg
  open: number; // index into OPEN
  closed: number; // index into CLOSED
}

function Eye({ spec, onGone }: { spec: EyeSpec; onGone: () => void }) {
  const [shut, setShut] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const gone = useRef(onGone);
  gone.current = onGone;

  useEffect(() => {
    let alive = true;
    const timers: number[] = [];
    const push = (t: number) => timers.push(t);

    // random blink loop - open for a while, snap shut for a beat, repeat
    const blink = () => {
      push(
        window.setTimeout(
          () => {
            if (!alive) return;
            setShut(true);
            push(
              window.setTimeout(() => {
                if (!alive) return;
                setShut(false);
                blink();
              }, 90 + Math.random() * 150),
            );
          },
          1400 + Math.random() * 4200,
        ),
      );
    };
    blink();

    // finite lifetime -> fade out -> ask the parent to remove us
    const life = 3500 + Math.random() * 7000;
    push(window.setTimeout(() => setLeaving(true), life));
    push(
      window.setTimeout(() => {
        alive = false;
        gone.current();
      }, life + 1000),
    );

    return () => {
      alive = false;
      timers.forEach(clearTimeout);
    };
  }, []);

  const Variant = shut ? CLOSED[spec.closed] : OPEN[spec.open];
  return (
    <div
      className={`creepy-eye ${leaving ? "creepy-eye-leaving" : ""}`}
      style={{
        left: `${spec.x}%`,
        top: `${spec.y}%`,
        width: spec.size,
        transform: `rotate(${spec.rot}deg)`,
      }}
    >
      <Variant className="creepy-eye-art" />
    </div>
  );
}

export default function CreepyEyes({ intensity }: { intensity: number }) {
  const [eyes, setEyes] = useState<EyeSpec[]>([]);
  const intensityRef = useRef(intensity);
  intensityRef.current = intensity;
  const nextId = useRef(1);

  // Director: on a steady tick, drift the population toward a hunger-scaled
  // target by trickling in eyes at random positions. Eyes remove themselves at
  // the end of their lifetime, so the count churns organically instead of
  // snapping to a number.
  useEffect(() => {
    const spawn = (): EyeSpec => ({
      id: nextId.current++,
      x: 1 + Math.random() * 93,
      y: 5 + Math.random() * 87,
      size: 42 + Math.random() * 104,
      rot: -22 + Math.random() * 44,
      open: Math.floor(Math.random() * OPEN.length),
      closed: Math.floor(Math.random() * CLOSED.length),
    });
    const tick = () => {
      setEyes((prev) => {
        // 0 hunger -> ~2 lurkers; 100 -> a crowd (~18)
        const target = 2 + Math.round((Math.max(0, intensityRef.current) / 100) * 16);
        if (prev.length >= target) {
          // occasional extra even at/over target keeps it alive
          return Math.random() < 0.12 ? [...prev, spawn()] : prev;
        }
        const toAdd = target - prev.length > 4 ? 2 : 1;
        return [...prev, ...Array.from({ length: toAdd }, spawn)];
      });
    };
    const interval = window.setInterval(tick, 420);
    return () => window.clearInterval(interval);
  }, []);

  const remove = useCallback((id: number) => {
    setEyes((prev) => prev.filter((e) => e.id !== id));
  }, []);

  return (
    <div className="creepy-eyes-layer" aria-hidden>
      {eyes.map((e) => (
        <Eye key={e.id} spec={e} onGone={() => remove(e.id)} />
      ))}
    </div>
  );
}
