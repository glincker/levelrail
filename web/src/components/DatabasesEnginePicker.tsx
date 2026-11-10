import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  ClockGlassIcon,
  DatabaseGlassIcon,
  KeyGlassIcon,
} from '@thesvg/ui-react'
import { BRAND_ICON_NAMES, BrandIcon } from '@/components/BrandIcon'
import { buildPickerEntries, type PickerEntry } from '@/lib/databaseCatalog'
import { useDatabaseEnginesOptional } from '../queries/databaseEngines'
import { FALLBACK_ENGINES } from './databaseFlow/logic'

// Explicit literal keys keep the typed t() happy.
const TAGLINE_KEYS = {
  postgres: 'picker.taglines.postgres',
  mysql: 'picker.taglines.mysql',
  mariadb: 'picker.taglines.mariadb',
  redis: 'picker.taglines.redis',
  valkey: 'picker.taglines.valkey',
  keydb: 'picker.taglines.keydb',
  dragonfly: 'picker.taglines.dragonfly',
  mongodb: 'picker.taglines.mongodb',
  clickhouse: 'picker.taglines.clickhouse',
  meilisearch: 'picker.taglines.meilisearch',
  qdrant: 'picker.taglines.qdrant',
  typesense: 'picker.taglines.typesense',
  memcached: 'picker.taglines.memcached',
} as const

const POPULAR = ['postgres', 'mysql', 'redis', 'mongodb', 'mariadb']

function isTaglineKey(key: string): key is keyof typeof TAGLINE_KEYS {
  return key in TAGLINE_KEYS
}

function orderEngines(entries: readonly PickerEntry[]): PickerEntry[] {
  const plain = entries.filter((e) => e.flavor === null)
  const rank = (e: PickerEntry) => {
    const i = POPULAR.indexOf(e.key)
    return i === -1 ? POPULAR.length : i
  }
  return plain.toSorted((a, b) => rank(a) - rank(b))
}

const VALUE_POINTS = [
  { key: 'credentials', icon: KeyGlassIcon },
  { key: 'backups', icon: ArchiveGlassIcon },
  { key: 'restore', icon: ClockGlassIcon },
] as const

export function DatabasesEnginePicker({
  onPick,
}: Readonly<{ onPick: (engineId: string) => void }>) {
  const { t } = useTranslation('databases')
  const query = useDatabaseEnginesOptional()
  const engines = query.data?.length ? query.data : FALLBACK_ENGINES
  const entries = orderEngines(buildPickerEntries(engines, BRAND_ICON_NAMES))
  return (
    <section aria-labelledby="db-engine-picker" className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h2 id="db-engine-picker" className="text-base font-semibold">
          {t('banner.pickEngine')}
        </h2>
        <p className="max-w-prose text-sm text-muted-foreground">
          {t('banner.whatFor')}
        </p>
      </div>
      <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {entries.map((entry) => (
          <li key={entry.key} className="flex">
            <button
              type="button"
              onClick={() => {
                onPick(entry.engineId)
              }}
              className="flex min-h-20 w-full items-center gap-3 rounded-container border border-border bg-card p-3 text-left shadow-card transition-colors hover:border-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring motion-reduce:transition-none"
            >
              <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-background">
                {entry.brand ? (
                  <BrandIcon name={entry.brand} className="size-6" />
                ) : (
                  <DatabaseGlassIcon
                    className="size-6 text-muted-foreground"
                    aria-hidden="true"
                  />
                )}
              </span>
              <span className="flex min-w-0 flex-col">
                <span className="text-sm font-medium">{entry.label}</span>
                <span className="line-clamp-2 text-xs text-muted-foreground">
                  {isTaglineKey(entry.key)
                    ? t(TAGLINE_KEYS[entry.key])
                    : t('picker.genericTagline', { label: entry.label })}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
      <ul className="flex flex-col gap-2 text-sm text-muted-foreground sm:flex-row sm:gap-6">
        {VALUE_POINTS.map(({ key, icon: Icon }) => (
          <li key={key} className="flex items-center gap-2">
            <Icon size="1.2em" aria-hidden="true" />
            {t(`banner.value.${key}`)}
          </li>
        ))}
      </ul>
    </section>
  )
}
