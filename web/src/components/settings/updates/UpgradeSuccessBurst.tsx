import { cn } from '@/lib/utils'

const PARTICLES = [
  'upg-a0',
  'upg-a1',
  'upg-a2',
  'upg-a3',
  'upg-a4',
  'upg-a5',
  'upg-a6',
  'upg-a7',
]

// One-shot radial pulse plus eight particles, CSS only, under 1.2s.
export function UpgradeSuccessBurst() {
  return (
    <span
      className="pointer-events-none absolute inset-0 grid place-items-center"
      aria-hidden
    >
      <span className="upg-burst absolute size-12 rounded-full bg-success/40" />
      {PARTICLES.map((a) => (
        <span
          key={a}
          className={cn(
            'upg-particle absolute size-1.5 rounded-full bg-success',
            a,
          )}
        />
      ))}
    </span>
  )
}
