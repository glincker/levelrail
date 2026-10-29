import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { SkeletonLine } from '@/components/kit'
import { stackRevisionQueryOptions } from '../../queries/stacks'
import type { IacSummary } from '../../queries/iac'
import type { StackDiffAgainst, StackResultBody } from '../../types/stacks'
import { RevisionStatus } from './StackIndicators'
import { ChangeList, StackDiffView } from './StackDiffView'

function SummaryLine({ summary }: Readonly<{ summary: IacSummary }>) {
  const { t } = useTranslation('stacks')
  return (
    <p className="text-sm text-muted-foreground">
      {t('revision.summary', { ...summary })}
    </p>
  )
}

function ResultSummary({ result }: Readonly<{ result: StackResultBody }>) {
  const { t } = useTranslation('stacks')
  const failed = (result.results ?? []).filter((r) => r.status === 'failed')
  return (
    <div className="space-y-1">
      <p className="text-sm text-muted-foreground">
        {t('revision.resultCounts', {
          applied: result.applied ?? 0,
          failed: result.failed ?? 0,
          skipped: result.skipped ?? 0,
        })}
      </p>
      {failed.length > 0 ? (
        <ul className="space-y-0.5 text-xs text-destructive">
          {failed.map((r) => (
            <li key={`${r.kind ?? ''}/${r.scope ?? ''}/${r.name ?? ''}`}>
              {t('revision.resultFailedItem', {
                kind: r.kind ?? '',
                name: r.name ?? '',
                reason: r.error ?? t('revision.resultNoReason'),
              })}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

export function RevisionView({
  name,
  revision,
  against,
  otherRevisions,
  showDiff,
  onAgainst,
  onClose,
}: Readonly<{
  name: string
  revision: number
  against: StackDiffAgainst
  otherRevisions: readonly number[]
  showDiff: boolean
  onAgainst: (next: StackDiffAgainst) => void
  onClose: () => void
}>) {
  const { t } = useTranslation('stacks')
  const { data, isPending, isError } = useQuery(
    stackRevisionQueryOptions(name, revision),
  )
  return (
    <section
      className="space-y-3 rounded-xl border border-border bg-card p-3 shadow-sm"
      aria-label={t('revision.title', { number: revision })}
    >
      <header className="flex items-center gap-2">
        <h3 className="text-sm font-semibold">
          {t('revision.title', { number: revision })}
        </h3>
        {data ? <RevisionStatus revision={data} /> : null}
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="ml-auto"
          aria-label={t('revision.close')}
          onClick={onClose}
        >
          <XGlassIcon size="1.2em" aria-hidden="true" />
        </Button>
      </header>
      {isPending ? (
        <SkeletonLine />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('revision.loadFailed')}</p>
      ) : (
        <>
          {showDiff ? (
            <StackDiffView
              name={name}
              revision={revision}
              against={against}
              otherRevisions={otherRevisions}
              onAgainst={onAgainst}
            />
          ) : null}
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <h4 className="text-xs font-medium uppercase text-muted-foreground">
                {t('revision.plan')}
              </h4>
              {data.plan?.summary ? (
                <SummaryLine summary={data.plan.summary} />
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t('revision.noPlan')}
                </p>
              )}
              {data.plan?.changes && data.plan.changes.length > 0 ? (
                <ChangeList changes={data.plan.changes} />
              ) : null}
              <p className="font-mono text-xs text-muted-foreground">
                {t('revision.hash', { hash: data.plan_hash.slice(0, 12) })}
              </p>
            </div>
            <div className="space-y-1.5">
              <h4 className="text-xs font-medium uppercase text-muted-foreground">
                {t('revision.result')}
              </h4>
              {data.result ? (
                <ResultSummary result={data.result} />
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t('revision.noResult')}
                </p>
              )}
            </div>
          </div>
          <div className="space-y-1.5">
            <h4 className="text-xs font-medium uppercase text-muted-foreground">
              {t('revision.files')}
            </h4>
            {data.files.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('revision.noFiles')}
              </p>
            ) : (
              data.files.map((f) => (
                <details
                  key={f.name}
                  className="rounded-lg border border-border"
                >
                  <summary className="cursor-pointer px-3 py-1.5 font-mono text-xs">
                    {f.name}
                  </summary>
                  <CodeBlock
                    code={f.content}
                    language="yaml"
                    className="border-0"
                  />
                </details>
              ))
            )}
          </div>
        </>
      )}
    </section>
  )
}
