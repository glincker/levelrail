export type IllustrationName =
  'rocket' | 'globe' | 'chart' | 'database' | 'cloud'

const common = {
  width: 112,
  height: 84,
  viewBox: '0 0 112 84',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
  'aria-hidden': true,
}

function Rocket() {
  return (
    <svg {...common} data-illustration="rocket">
      <circle
        cx="20"
        cy="18"
        r="1.5"
        fill="currentColor"
        stroke="none"
        opacity="0.4"
      />
      <circle
        cx="94"
        cy="26"
        r="2"
        fill="currentColor"
        stroke="none"
        opacity="0.3"
      />
      <circle
        cx="84"
        cy="10"
        r="1.5"
        fill="currentColor"
        stroke="none"
        opacity="0.4"
      />
      <path
        d="M56 8c12 8 18 22 16 40H40C38 30 44 16 56 8Z"
        fill="currentColor"
        fillOpacity="0.08"
      />
      <circle cx="56" cy="30" r="6" />
      <path d="M40 40l-10 12 10-2M72 40l10 12-10-2" />
      <path d="M50 58c0 8 3 14 6 18 3-4 6-10 6-18" opacity="0.6" />
    </svg>
  )
}

function Globe() {
  return (
    <svg {...common} data-illustration="globe">
      <circle cx="56" cy="42" r="28" fill="currentColor" fillOpacity="0.08" />
      <ellipse cx="56" cy="42" rx="12" ry="28" />
      <path d="M28 42h56M32 28h48M32 56h48" opacity="0.6" />
      <circle
        cx="92"
        cy="18"
        r="3"
        fill="currentColor"
        stroke="none"
        opacity="0.4"
      />
      <circle
        cx="18"
        cy="66"
        r="2"
        fill="currentColor"
        stroke="none"
        opacity="0.3"
      />
    </svg>
  )
}

function Chart() {
  return (
    <svg {...common} data-illustration="chart">
      <path d="M16 68h80" opacity="0.5" />
      <rect
        x="24"
        y="44"
        width="12"
        height="24"
        rx="3"
        fill="currentColor"
        fillOpacity="0.08"
      />
      <rect
        x="44"
        y="30"
        width="12"
        height="38"
        rx="3"
        fill="currentColor"
        fillOpacity="0.08"
      />
      <rect
        x="64"
        y="38"
        width="12"
        height="30"
        rx="3"
        fill="currentColor"
        fillOpacity="0.08"
      />
      <path d="M24 30c12-4 20-14 32-14s18 8 40 2" opacity="0.7" />
      <circle cx="96" cy="18" r="3" fill="currentColor" stroke="none" />
    </svg>
  )
}

function Database() {
  return (
    <svg {...common} data-illustration="database">
      <ellipse
        cx="56"
        cy="20"
        rx="26"
        ry="9"
        fill="currentColor"
        fillOpacity="0.08"
      />
      <path d="M30 20v40c0 5 12 9 26 9s26-4 26-9V20" />
      <path d="M30 40c0 5 12 9 26 9s26-4 26-9" opacity="0.6" />
      <circle cx="74" cy="58" r="1.5" fill="currentColor" stroke="none" />
    </svg>
  )
}

function Cloud() {
  return (
    <svg {...common} data-illustration="cloud">
      <path
        d="M34 62a14 14 0 0 1-2-27.8A20 20 0 0 1 70 30a16 16 0 0 1 8 32H34Z"
        fill="currentColor"
        fillOpacity="0.08"
      />
      <path d="M56 70V46m-7 7 7-7 7 7" opacity="0.7" />
      <circle
        cx="92"
        cy="16"
        r="2"
        fill="currentColor"
        stroke="none"
        opacity="0.4"
      />
    </svg>
  )
}

const MAP: Record<IllustrationName, () => React.JSX.Element> = {
  rocket: Rocket,
  globe: Globe,
  chart: Chart,
  database: Database,
  cloud: Cloud,
}

export function Illustration({ name }: { name: IllustrationName }) {
  const C = MAP[name]
  return <C />
}
