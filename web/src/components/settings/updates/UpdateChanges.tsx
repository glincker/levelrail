import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, ChevronRightGlassIcon } from '@thesvg/ui-react'
import { preflightQueryOptions, type UpdateStatus } from '@/queries/updates'
import { ReleaseNotes } from '../ReleaseNotes'

// UpdateChanges previews an available update: the version delta, release
// date, collapsed notes, and the existing backup/restart/rollback promises.
export function UpdateChanges({ status }: { status: UpdateStatus }) {
  const { t } = useTranslation('updates')
  const preflight = useQuery(preflightQueryOptions())
  const [open, setOpen] = useState(false)
  const notes = preflight.data?.release_notes ?? ''
  const published = status.published_at ?? preflight.data?.release_published_at
  const released = published
    ? new Date(published).toLocaleDateString()
    : t('changes.releasedUnknown')

  return (
    <section
      aria-label={t('changes.title')}
      className="space-y-4 rounded-lg border border-border bg-muted/30 p-4"
    >
      <h3 className="text-sm font-medium">{t('changes.title')}</h3>
      <dl className="grid gap-3 text-sm sm:grid-cols-2">
        <div className="space-y-1">
          <dt className="text-xs text-muted-foreground">
            {t('changes.versionLabel')}
          </dt>
          <dd className="flex flex-wrap items-center gap-1.5 font-mono text-xs">
            <code className="rounded bg-muted px-1.5 py-0.5">
              {status.current_version}
            </code>
            <span aria-hidden="true">{'>'}</span>
            <code className="rounded bg-muted px-1.5 py-0.5 text-foreground">
              {status.latest_version}
            </code>
          </dd>
        </div>
        <div className="space-y-1">
          <dt className="text-xs text-muted-foreground">
            {t('changes.releasedLabel')}
          </dt>
          <dd>{released}</dd>
        </div>
      </dl>
      <ul className="max-w-prose list-disc space-y-1 pl-5 text-sm text-muted-foreground">
        <li>{t('upgradeNow.willBackup')}</li>
        <li>{t('upgradeNow.willRestart')}</li>
        <li>{t('upgradeNow.willRollback')}</li>
      </ul>
      <div className="space-y-2">
        <button
          type="button"
          aria-expanded={open}
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          onClick={() => {
            setOpen((v) => !v)
          }}
        >
          {open ? (
            <ChevronDownGlassIcon className="size-5" />
          ) : (
            <ChevronRightGlassIcon className="size-5" />
          )}
          {t('changes.notesToggle')}
        </button>
        {open ? (
          notes !== '' ? (
            <div className="max-h-80 overflow-y-auto rounded-md border border-border bg-background p-3">
              <ReleaseNotes markdown={notes} />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('changes.notesEmpty')}
            </p>
          )
        ) : null}
      </div>
    </section>
  )
}
