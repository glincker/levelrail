import { useTranslation } from 'react-i18next'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  POSTGRES_FLAVORS,
  flavorOfVersion,
  withFlavor,
  type PostgresFlavor,
} from '@/lib/databaseCatalog'

const NONE = 'none'

const IMAGE_BY_FLAVOR: Record<PostgresFlavor, string> = {
  pgvector: 'pgvector/pgvector',
  timescaledb: 'timescale/timescaledb',
  postgis: 'postgis/postgis',
}

// Picks which allowlisted Postgres image the version string selects; the
// version stays the single source of truth so the form needs no new field.
export function PostgresFlavorField({
  version,
  onVersionChange,
}: {
  version: string
  onVersionChange: (next: string) => void
}) {
  const { t } = useTranslation('databases')
  const flavor = flavorOfVersion(version)

  const optionLabels: Record<PostgresFlavor, string> = {
    pgvector: t('createFlavor.options.pgvector'),
    timescaledb: t('createFlavor.options.timescaledb'),
    postgis: t('createFlavor.options.postgis'),
  }

  return (
    <Field>
      <FieldLabel htmlFor="database-flavor">
        {t('createFlavor.label')}
      </FieldLabel>
      <Select
        value={flavor ?? NONE}
        onValueChange={(next) => {
          const picked = POSTGRES_FLAVORS.find((f) => f === next) ?? null
          onVersionChange(withFlavor(version, picked))
        }}
      >
        <SelectTrigger id="database-flavor" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE}>{t('createFlavor.none')}</SelectItem>
          {POSTGRES_FLAVORS.map((f) => (
            <SelectItem key={f} value={f}>
              {optionLabels[f]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <FieldHint>
        {flavor
          ? t('createFlavor.hint', {
              image: IMAGE_BY_FLAVOR[flavor],
              version,
            })
          : t('createFlavor.hintNone')}
      </FieldHint>
    </Field>
  )
}
