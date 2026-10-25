import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SkeletonLine } from '@/components/kit'
import { stackDiffQueryOptions } from '../../queries/stacks'
import type { IacAction, IacChange } from '../../queries/iac'
import type { StackDiffAgainst } from '../../types/stacks'

const ACTION_VARIANT: Record<
  IacAction,
  'success' | 'info' | 'destructive' | 'muted' | 'warning'
> = {
  create: 'success',
  update: 'info',
  delete: 'destructive',
  noop: 'muted',
  error: 'warning',
}

export function ChangeList({
  changes,
}: Readonly<{ changes: readonly IacChange[] }>) {
  const { t } = useTranslation('stacks')
  return (
    <ul className="divide-y divide-border rounded-lg border border-border">
      {changes.map((c) => (
        <li
          key={`${c.kind}/${c.scope ?? ''}/${c.name}`}
          className="space-y-1 px-3 py-2"
        >
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={ACTION_VARIANT[c.action]}>
              {t(`diff.action.${c.action}`)}
            </Badge>
            <span className="font-mono text-xs text-muted-foreground">
              {c.kind}
            </span>
            <span className="font-medium">{c.name}</span>
          </div>
          {c.fields && c.fields.length > 0 ? (
            <ul className="space-y-0.5 font-mono text-xs">
              {c.fields.map((f) => (
                <li key={f.path} className="flex flex-wrap gap-x-2">
                  <span className="text-muted-foreground">{f.path}</span>
                  {f.old !== undefined ? (
                    <span className="text-tone-danger line-through">
                      {f.old}
                    </span>
                  ) : null}
                  {f.new !== undefined ? (
                    <span className="text-tone-success">{f.new}</span>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

export function StackDiffView({
  name,
  revision,
  against,
  otherRevisions,
  onAgainst,
}: Readonly<{
  name: string
  revision: number
  against: StackDiffAgainst
  otherRevisions: readonly number[]
  onAgainst: (next: StackDiffAgainst) => void
}>) {
  const { t } = useTranslation('stacks')
  const { data, isPending, isError } = useQuery(
    stackDiffQueryOptions(name, revision, against),
  )
  const options: StackDiffAgainst[] = ['live', ...otherRevisions]
  return (
    <section className="space-y-2" aria-label={t('diff.title')}>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-sm text-muted-foreground">
          {t('diff.against', { number: revision })}
        </span>
        {options.map((o) => (
          <Button
            key={String(o)}
            type="button"
            size="xs"
            variant={o === against ? 'default' : 'outline'}
            aria-pressed={o === against}
            onClick={() => {
              onAgainst(o)
            }}
          >
            {o === 'live'
              ? t('diff.live')
              : t('detail.revisionShort', { number: o })}
          </Button>
        ))}
      </div>
      {isPending ? (
        <SkeletonLine />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('diff.loadFailed')}</p>
      ) : data.changes.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('diff.empty')}</p>
      ) : (
        <ChangeList changes={data.changes} />
      )}
    </section>
  )
}
