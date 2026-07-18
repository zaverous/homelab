import { useId } from "react";

export default function HungerScratch({ value, critical = false }: { value: number; critical?: boolean }) {
  const clipId = useId().replaceAll(":", "");
  const bounded = Math.max(0, Math.min(100, value));
  const width = 440 * bounded / 100;

  return (
    <svg
      className={`hunger-scratch ${critical ? "hunger-scratch-critical" : ""}`}
      viewBox="0 0 440 42"
      role="img"
      aria-label={`Hunger ${bounded} out of 100`}
      preserveAspectRatio="none"
    >
      <defs>
        <clipPath id={clipId}>
          <rect x="0" y="0" width={width} height="42" />
        </clipPath>
      </defs>

      <g className="hunger-track-art" fill="none" stroke="currentColor" strokeLinecap="round">
        <path d="M4 24C29 22 47 27 72 23S116 26 143 22s45 4 72 0 46 3 72 0 50 4 75 0 45 2 74-1" />
        <path d="M8 27c36-2 64 2 101-1s67 2 102-1 69 2 104-1 71 1 117-1" opacity=".32" />
        <path d="M26 20l-8 10m69-11-5 13m77-13-7 14m76-14-4 13m77-14-6 14m76-13-5 11m50-11-3 9" opacity=".48" />
      </g>

      <g className="hunger-progress-art" clipPath={`url(#${clipId})`} fill="none" stroke="currentColor" strokeLinecap="round">
        <path d="M4 24C29 22 47 27 72 23S116 26 143 22s45 4 72 0 46 3 72 0 50 4 75 0 45 2 74-1" />
        <path d="M8 27c36-2 64 2 101-1s67 2 102-1 69 2 104-1 71 1 117-1" opacity=".75" />
      </g>
    </svg>
  );
}
