import { useTranslation } from 'react-i18next'
import { ShareGlassIcon } from '@thesvg/ui-react'
import { BrandIcon } from '../BrandIcon'
import { KIND_MARKS } from '../../lib/dataExportKinds'
import { KIND_KEYS, KINDS } from '../../lib/dataExportForm'
import type { DataExportKind } from '../../types/dataExport'

const GROUPS = ['logs', 'audit', 'metrics'] as const

export function KindMark({ kind }: Readonly<{ kind: DataExportKind }>) {
  const mark = KIND_MARKS[kind]
  return mark ? (
    <BrandIcon name={mark} className="size-5 shrink-0" />
  ) : (
    <ShareGlassIcon className="size-5 shrink-0" aria-hidden="true" />
  )
}

/** DestinationGallery lists every destination type the backend accepts, grouped by what it carries. */
export function DestinationGallery({
  onPick,
}: Readonly<{ onPick: (kind: DataExportKind) => void }>) {
  const { t } = useTranslation('dataExport')
  return (
    <div className="space-y-3">
      {GROUPS.map((g) => (
        <section key={g} aria-label={t(`gallery.${g}`)}>
          <h3 className="mb-1.5 text-xs font-medium text-muted-foreground">
            {t(`gallery.${g}`)}
          </h3>
          <ul className="grid gap-2 sm:grid-cols-2">
            {KINDS.filter((k) => k.startsWith(g)).map((k) => (
              <li key={k}>
                <button
                  type="button"
                  onClick={() => {
                    onPick(k)
                  }}
                  className="flex w-full items-center gap-2 rounded-md border px-2.5 py-2 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  <KindMark kind={k} />
                  <span className="min-w-0">{t(KIND_KEYS[k])}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  )
}
