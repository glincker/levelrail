import { Display } from '@/components/ui/typography'
import { useEffect, useState } from 'react'
import {
  CheckGlassIcon,
  CircleCheckGlassIcon,
  DatabaseGlassIcon,
  GitBranchGlassIcon,
  LockGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { useReducedMotion } from './kit/useReducedMotion'
import { cn } from '@/lib/utils'

const GRAIN =
  "url(\"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='220' height='220'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2'/><feColorMatrix values='0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 9 0 0 0 -4.3'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>\")"

const GLOW =
  'radial-gradient(54% 92% at 50% 0%, rgba(182,171,255,.50), transparent 72%), radial-gradient(38% 78% at 10% 0%, rgba(0,203,170,.42), transparent 72%), radial-gradient(36% 70% at 92% 0%, rgba(238,243,95,.34), transparent 72%)'

const CARD_STYLE: React.CSSProperties = {
  background:
    'linear-gradient(180deg, rgba(255,255,255,.035), rgba(255,255,255,0)) padding-box, linear-gradient(#0e0f11,#0e0f11) padding-box, linear-gradient(180deg, rgba(255,255,255,.18), rgba(255,255,255,.04) 40%, rgba(255,255,255,.06)) border-box',
  boxShadow:
    'inset 0 1px 0 rgba(255,255,255,.07), 0 1px 1px rgba(0,0,0,.5), 0 8px 16px -4px rgba(0,0,0,.55), 0 24px 48px -12px rgba(0,0,0,.65)',
}

const ROTATE_MS = 7000
const PROOF = [
  'Your servers',
  'Git push deploys',
  'Managed databases',
  'Auto TLS',
]

function Reveal({
  active,
  step,
  className,
  children,
}: {
  active: boolean
  step: number
  className?: string
  children: React.ReactNode
}) {
  return (
    <div
      className={cn(
        'transition-[opacity,transform] duration-500 ease-out motion-reduce:transition-none',
        active ? 'translate-y-0 opacity-100' : 'translate-y-2 opacity-0',
        className,
      )}
      style={{ transitionDelay: active ? `${250 + step * 350}ms` : '0ms' }}
    >
      {children}
    </div>
  )
}

function MockCard({ children }: { children: React.ReactNode }) {
  return (
    <div
      aria-hidden="true"
      className="w-full max-w-md rounded-xl border border-transparent p-4 font-mono text-xs text-[#cfcfd3]"
      style={CARD_STYLE}
    >
      {children}
    </div>
  )
}

function MockRow({
  active,
  step,
  icon,
  label,
  value,
}: {
  active: boolean
  step: number
  icon: React.ReactNode
  label: string
  value: string
}) {
  return (
    <Reveal active={active} step={step} className="flex items-center gap-2">
      {icon}
      <span className="flex-1 text-white">{label}</span>
      <span className="text-[#8d8d96]">{value}</span>
    </Reveal>
  )
}

const OK = <CircleCheckGlassIcon className="size-4.5 text-[#00cbaa]" />

function DeployVisual({ active }: { active: boolean }) {
  return (
    <MockCard>
      <p className="flex items-center gap-2 text-[#8d8d96]">
        <GitBranchGlassIcon className="size-4" />
        <span>$ git push</span>
      </p>
      <div className="mt-4 space-y-2.5">
        <MockRow
          active={active}
          step={0}
          icon={OK}
          label="Build"
          value="12m 40s"
        />
        <MockRow
          active={active}
          step={1}
          icon={OK}
          label="Roll out"
          value="38s"
        />
        <MockRow
          active={active}
          step={2}
          icon={OK}
          label="TLS"
          value="issued"
        />
      </div>
      <Reveal
        active={active}
        step={3}
        className="mt-4 flex items-center gap-2 border-t border-white/[0.07] pt-3 text-[#8d8d96]"
      >
        <span className="size-1.5 rounded-full bg-[#00cbaa]" />
        my-app is live
      </Reveal>
    </MockCard>
  )
}

function DatabaseVisual({ active }: { active: boolean }) {
  const icon = <DatabaseGlassIcon className="size-4.5 text-[#b6abff]" />
  return (
    <MockCard>
      <p className="text-[#8d8d96]">Managed databases</p>
      <div className="mt-4 space-y-2.5">
        <MockRow
          active={active}
          step={0}
          icon={icon}
          label="postgres"
          value="healthy"
        />
        <MockRow
          active={active}
          step={1}
          icon={icon}
          label="redis"
          value="healthy"
        />
        <MockRow
          active={active}
          step={2}
          icon={icon}
          label="backups"
          value="scheduled"
        />
      </div>
      <Reveal
        active={active}
        step={3}
        className="mt-4 border-t border-white/[0.07] pt-3 text-[#8d8d96]"
      >
        Restore to a point in time
      </Reveal>
    </MockCard>
  )
}

function NodesVisual({ active }: { active: boolean }) {
  const bars = [38, 62, 45, 80, 55, 70, 48, 66]
  return (
    <MockCard>
      <p className="text-[#8d8d96]">Nodes</p>
      <div className="mt-4 space-y-3">
        {['node-a', 'node-b', 'node-c'].map((node, i) => (
          <Reveal
            key={node}
            active={active}
            step={i}
            className="flex items-center gap-3"
          >
            <ServerGlassIcon className="size-4.5 text-[#eef35f]" />
            <span className="w-14 text-white">{node}</span>
            <span className="flex h-5 flex-1 items-end gap-1">
              {bars.map((h, j) => (
                <span
                  key={j}
                  className="flex-1 rounded-sm bg-[#00cbaa]/70 transition-[height] duration-700 ease-out motion-reduce:transition-none"
                  style={{
                    height: active
                      ? `${((h + i * 9 + j * 3) % 70) + 25}%`
                      : '10%',
                    transitionDelay: `${300 + j * 60}ms`,
                  }}
                />
              ))}
            </span>
          </Reveal>
        ))}
      </div>
    </MockCard>
  )
}

function TlsVisual({ active }: { active: boolean }) {
  return (
    <MockCard>
      <Reveal active={active} step={0} className="flex items-center gap-2">
        <LockGlassIcon className="size-4.5 text-[#00cbaa]" />
        <span className="text-white">app.example.com</span>
      </Reveal>
      <div className="mt-4 space-y-2.5">
        <MockRow
          active={active}
          step={1}
          icon={OK}
          label="DNS"
          value="verified"
        />
        <MockRow
          active={active}
          step={2}
          icon={OK}
          label="Certificate"
          value="issued"
        />
        <MockRow
          active={active}
          step={3}
          icon={OK}
          label="Renewal"
          value="automatic"
        />
      </div>
    </MockCard>
  )
}

interface Slide {
  eyebrow: string
  headline: React.ReactNode
  points: string[]
  Visual: (props: { active: boolean }) => React.ReactNode
}

const SLIDES: Slide[] = [
  {
    eyebrow: 'Self-hosted deploys',
    headline: (
      <>
        Ship to your own servers,
        <br />
        <span className="text-[#eef35f]">without the glue.</span>
      </>
    ),
    points: [
      'Push to git, get a build and a rollout.',
      'Failed deploys explain themselves.',
      'Roll back to any earlier release in a click.',
    ],
    Visual: DeployVisual,
  },
  {
    eyebrow: 'Data next door',
    headline: (
      <>
        Databases that live
        <br />
        <span className="text-[#eef35f]">beside your apps.</span>
      </>
    ),
    points: [
      'Postgres and Redis, provisioned and wired in.',
      'Scheduled backups and point in time restore.',
      'Connection strings injected, never pasted.',
    ],
    Visual: DatabaseVisual,
  },
  {
    eyebrow: 'One control plane',
    headline: (
      <>
        Every node,
        <br />
        <span className="text-[#eef35f]">one dashboard.</span>
      </>
    ),
    points: [
      'See load and health across all your servers.',
      'Place apps where they fit.',
      'An audit trail for every change.',
    ],
    Visual: NodesVisual,
  },
  {
    eyebrow: 'Secure by default',
    headline: (
      <>
        HTTPS the moment
        <br />
        <span className="text-[#eef35f]">a domain points here.</span>
      </>
    ),
    points: [
      'Certificates issue and renew on their own.',
      'Passkeys and one time codes for sign in.',
      'Secrets stay encrypted at rest.',
    ],
    Visual: TlsVisual,
  },
]

// Always dark, like the TheAuth site it borrows from, in both app themes.
export function LoginBrandPanel({ name }: { name: string }) {
  const reduced = useReducedMotion()
  const [index, setIndex] = useState(0)

  useEffect(() => {
    if (reduced) return
    const timer = window.setTimeout(() => {
      setIndex((i) => (i + 1) % SLIDES.length)
    }, ROTATE_MS)
    return () => {
      window.clearTimeout(timer)
    }
  }, [index, reduced])

  return (
    <aside
      aria-label={`About ${name}`}
      className="relative isolate hidden flex-col justify-between gap-10 overflow-hidden bg-black p-10 text-[#cfcfd3] lg:flex xl:p-14"
    >
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[34rem] opacity-[0.42]"
        style={{ background: GLOW }}
      />
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[34rem]"
        style={{
          background: GLOW,
          filter: 'saturate(1.2) brightness(1.25)',
          maskImage: `${GRAIN}, linear-gradient(to bottom, #000 0%, transparent 88%)`,
          maskSize: '220px 220px, 100% 100%',
          maskComposite: 'intersect',
          WebkitMaskImage: `${GRAIN}, linear-gradient(to bottom, #000 0%, transparent 88%)`,
          WebkitMaskSize: '220px 220px, 100% 100%',
          WebkitMaskComposite: 'source-in',
        }}
      />

      <div className="mt-auto grid">
        {SLIDES.map((slide, i) => {
          const active = i === index
          return (
            <div
              key={slide.eyebrow}
              aria-hidden={!active}
              className={cn(
                'col-start-1 row-start-1 space-y-8 transition-opacity duration-700 motion-reduce:transition-none',
                active ? 'opacity-100' : 'pointer-events-none opacity-0',
              )}
            >
              <div className="space-y-5">
                <p className="flex items-center gap-2 font-mono text-xs tracking-[0.08em] text-[#8d8d96] uppercase">
                  <span
                    aria-hidden="true"
                    className="size-1.5 rounded-full bg-[#00cbaa]"
                  />
                  {slide.eyebrow}
                </p>
                <Display
                  as="h2"
                  size="page"
                  className="max-w-xl text-[clamp(2.25rem,3.6vw,3.5rem)] leading-[1.05] font-light tracking-[-0.025em] text-white"
                >
                  {slide.headline}
                </Display>
              </div>
              <ul className="max-w-md space-y-2.5">
                {slide.points.map((point) => (
                  <li
                    key={point}
                    className="grid grid-cols-[1rem_1fr] gap-3 text-base"
                  >
                    <CheckGlassIcon
                      className="mt-0.5 size-4.5 text-[#00cbaa]"
                      aria-hidden="true"
                    />
                    <span>{point}</span>
                  </li>
                ))}
              </ul>
              <slide.Visual active={active} />
            </div>
          )
        })}
      </div>

      <div className="space-y-4">
        <div className="flex gap-2">
          {SLIDES.map((slide, i) => (
            <button
              key={slide.eyebrow}
              type="button"
              aria-current={i === index ? 'true' : undefined}
              aria-label={`Show: ${slide.eyebrow}`}
              onClick={() => {
                setIndex(i)
              }}
              className="group relative h-6 w-12 outline-none"
            >
              <span className="absolute inset-x-0 top-1/2 h-0.5 -translate-y-1/2 overflow-hidden rounded-full bg-white/15">
                <span
                  className="block h-full rounded-full bg-white ease-linear"
                  style={{
                    width: i === index ? '100%' : i < index ? '100%' : '0%',
                    opacity: i < index ? 0.35 : 1,
                    transition:
                      i === index && !reduced
                        ? `width ${ROTATE_MS}ms linear`
                        : 'none',
                  }}
                />
              </span>
              <span className="absolute inset-0 rounded group-focus-visible:ring-2 group-focus-visible:ring-[#b6abff]" />
            </button>
          ))}
        </div>
        <ul className="flex flex-wrap gap-y-2 border-t border-white/[0.07] pt-4 font-mono text-xs text-[#8d8d96]">
          {PROOF.map((item, i) => (
            <li
              key={item}
              className={i === 0 ? 'pr-4' : 'border-l border-white/[0.07] px-4'}
            >
              {item}
            </li>
          ))}
        </ul>
      </div>
    </aside>
  )
}
