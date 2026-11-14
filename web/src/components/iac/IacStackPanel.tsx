import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SkeletonLine } from '@/components/kit'
import {
  stackHistoryQueryOptions,
  stacksQueryOptions,
} from '../../queries/stacks'
import { IacRevisionTimeline } from './IacRevisionTimeline'

// Stack history next to the editor: the first stack is preselected, its revisions read as a timeline.
export function IacStackPanel() {
  const { t } = useTranslation('settings')
  const selectId = useId()
  const stacks = useQuery(stacksQueryOptions())
  const [picked, setPicked] = useState<string | null>(null)
  const [revision, setRevision] = useState<number | null>(null)
  const list = stacks.data ?? []
  const name =
    picked && list.some((s) => s.name === picked) ? picked : list[0]?.name
  const history = useQuery({
    ...stackHistoryQueryOptions(name ?? ''),
    enabled: Boolean(name),
  })
  const stack = list.find((s) => s.name === name)

  if (stacks.isPending) return <SkeletonLine />
  if (stacks.isError || list.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('iacStudio.stack.none')}
      </p>
    )
  }
  const revisions = history.data?.revisions ?? []
  const current = revision ?? revisions[0]?.number ?? null

  return (
    <section
      aria-label={t('iacStudio.stack.title')}
      className="space-y-2 rounded-md border p-3"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-sm font-medium">{t('iacStudio.stack.title')}</h3>
        <label htmlFor={selectId} className="sr-only">
          {t('iacStudio.stack.pick')}
        </label>
        <select
          id={selectId}
          value={name}
          onChange={(e) => {
            setPicked(e.target.value)
            setRevision(null)
          }}
          className="h-8 rounded-md border bg-background px-2 text-sm"
        >
          {list.map((s) => (
            <option key={s.name} value={s.name}>
              {s.name}
            </option>
          ))}
        </select>
      </div>
      {history.isPending ? (
        <SkeletonLine />
      ) : history.isError ? (
        <p className="text-sm text-tone-danger">
          {t('iacStudio.stack.loadFailed')}
        </p>
      ) : (
        <IacRevisionTimeline
          revisions={revisions}
          pinned={stack?.pinned_revision}
          selected={current}
          onSelect={setRevision}
        />
      )}
    </section>
  )
}
