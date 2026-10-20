// Static class strings so Tailwind's scanner keeps them; one accent per engine.
export interface EngineAccent {
  /** Gradient wash behind the step header. */
  wash: string
  /** Soft blurred orb in the header's corner. */
  orb: string
  ring: string
}

const NEUTRAL: EngineAccent = {
  wash: 'from-primary/10',
  orb: 'bg-primary/15',
  ring: 'ring-border',
}

const ACCENTS: Record<string, EngineAccent> = {
  postgres: {
    wash: 'from-sky-500/15',
    orb: 'bg-sky-500/20',
    ring: 'ring-sky-500/25',
  },
  mysql: {
    wash: 'from-cyan-600/15',
    orb: 'bg-cyan-600/20',
    ring: 'ring-cyan-600/25',
  },
  mariadb: {
    wash: 'from-amber-700/15',
    orb: 'bg-amber-700/20',
    ring: 'ring-amber-700/25',
  },
  redis: {
    wash: 'from-red-500/15',
    orb: 'bg-red-500/20',
    ring: 'ring-red-500/25',
  },
  valkey: {
    wash: 'from-indigo-500/15',
    orb: 'bg-indigo-500/20',
    ring: 'ring-indigo-500/25',
  },
  keydb: {
    wash: 'from-rose-500/15',
    orb: 'bg-rose-500/20',
    ring: 'ring-rose-500/25',
  },
  dragonfly: {
    wash: 'from-violet-500/15',
    orb: 'bg-violet-500/20',
    ring: 'ring-violet-500/25',
  },
  mongodb: {
    wash: 'from-green-500/15',
    orb: 'bg-green-500/20',
    ring: 'ring-green-500/25',
  },
  clickhouse: {
    wash: 'from-yellow-400/20',
    orb: 'bg-yellow-400/25',
    ring: 'ring-yellow-400/30',
  },
  timescaledb: {
    wash: 'from-amber-400/20',
    orb: 'bg-amber-400/25',
    ring: 'ring-amber-400/30',
  },
  pgvector: {
    wash: 'from-fuchsia-500/15',
    orb: 'bg-fuchsia-500/20',
    ring: 'ring-fuchsia-500/25',
  },
  postgis: {
    wash: 'from-emerald-500/15',
    orb: 'bg-emerald-500/20',
    ring: 'ring-emerald-500/25',
  },
  meilisearch: {
    wash: 'from-pink-500/15',
    orb: 'bg-pink-500/20',
    ring: 'ring-pink-500/25',
  },
  qdrant: {
    wash: 'from-red-600/15',
    orb: 'bg-red-600/20',
    ring: 'ring-red-600/25',
  },
  typesense: {
    wash: 'from-orange-500/15',
    orb: 'bg-orange-500/20',
    ring: 'ring-orange-500/25',
  },
  memcached: {
    wash: 'from-teal-500/15',
    orb: 'bg-teal-500/20',
    ring: 'ring-teal-500/25',
  },
}

/** engineAccent returns the brand accent for an engine or flavor key. */
export function engineAccent(key: string | null): EngineAccent {
  return (key !== null ? ACCENTS[key] : undefined) ?? NEUTRAL
}
