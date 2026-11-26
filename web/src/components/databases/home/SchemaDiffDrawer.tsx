import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { ApiError } from '@/lib/apiError'
import {
  useBranchSchemaDiff,
  type BranchResource,
  type SchemaDiffChange,
} from '../../../queries/databaseBranches'

const KIND_STYLE: Record<SchemaDiffChange['kind'], string> = {
  added: 'text-tone-success',
  removed: 'text-tone-danger',
  changed: 'text-tone-warning',
}
const KIND_MARK: Record<SchemaDiffChange['kind'], string> = {
  added: '+',
  removed: '-',
  changed: '~',
}
const KINDS = ['added', 'removed', 'changed'] as const

function changeLabel(c: SchemaDiffChange): string {
  return [c.schema, c.table, c.name].filter(Boolean).join('.')
}

/** SchemaDiffDrawer shows what one branch changed against its parent, with a migration to review. */
export function SchemaDiffDrawer({
  database,
  branch,
  branches,
  onClose,
}: Readonly<{
  database: string
  branch: string | null
  branches: BranchResource[]
  onClose: () => void
}>) {
  const { t } = useTranslation('dbBranches')
  const [against, setAgainst] = useState('')
  const diff = useBranchSchemaDiff(database, branch ?? '', against, !!branch)
  const options = branches.filter((b) => b.name !== branch)
  const notReady = diff.error instanceof ApiError && diff.error.status === 409

  function copySql() {
    if (!diff.data) return
    void navigator.clipboard.writeText(diff.data.sql).then(() => {
      toast.add({ title: t('schemaDiff.copied'), type: 'success' })
    })
  }

  return (
    <Sheet
      open={!!branch}
      onOpenChange={(open) => {
        if (!open) {
          setAgainst('')
          onClose()
        }
      }}
    >
      <SheetContent className="overflow-y-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>{t('schemaDiff.title')}</SheetTitle>
          <SheetDescription>
            {t('schemaDiff.description', {
              branch,
              against: diff.data?.against ?? t('schemaDiff.parent'),
            })}
          </SheetDescription>
        </SheetHeader>
        <div className="space-y-4 px-4 pb-4">
          <label className="flex items-center gap-2 text-sm">
            {t('schemaDiff.against')}
            <NativeSelect
              value={against}
              onChange={(e) => {
                setAgainst(e.target.value)
              }}
            >
              <option value="">{t('schemaDiff.parent')}</option>
              {options.map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
            </NativeSelect>
          </label>
          {diff.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : notReady ? (
            <Alert>
              <AlertDescription>{t('notReady')}</AlertDescription>
            </Alert>
          ) : diff.error ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('schemaDiff.loadFailed', { error: diff.error.message })}
              </AlertDescription>
            </Alert>
          ) : diff.data ? (
            <>
              <ul className="flex flex-wrap gap-4 text-sm">
                {KINDS.map((k) => (
                  <li key={k} className={KIND_STYLE[k]}>
                    {t(`schemaDiff.${k}`, { count: diff.data.summary[k] })}
                  </li>
                ))}
              </ul>
              {diff.data.truncated ? (
                <Alert>
                  <AlertDescription>
                    {t('schemaDiff.truncated')}
                  </AlertDescription>
                </Alert>
              ) : null}
              {diff.data.changes.length === 0 ? (
                <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                  {t('schemaDiff.none')}
                </p>
              ) : (
                <div className="space-y-3">
                  {KINDS.map((k) => {
                    const rows = diff.data.changes.filter((c) => c.kind === k)
                    return rows.length === 0 ? null : (
                      <section key={k} aria-label={t(`schemaDiff.group.${k}`)}>
                        <h3 className="mb-1 text-xs font-medium text-muted-foreground uppercase">
                          {t(`schemaDiff.group.${k}`)}
                        </h3>
                        <ul className="divide-y rounded-lg border font-mono text-xs">
                          {rows.map((c) => (
                            <li
                              key={`${c.object}:${changeLabel(c)}`}
                              className={`flex flex-wrap gap-x-2 px-3 py-1.5 ${KIND_STYLE[k]}`}
                            >
                              <span aria-hidden="true">{KIND_MARK[k]}</span>
                              <span className="text-muted-foreground">
                                {t(`schemaDiff.object.${c.object}`)}
                              </span>
                              <span className="break-all">
                                {changeLabel(c)}
                              </span>
                              {c.before && c.after ? (
                                <span className="w-full break-all text-muted-foreground">
                                  {c.before} {'->'} {c.after}
                                </span>
                              ) : null}
                            </li>
                          ))}
                        </ul>
                      </section>
                    )
                  })}
                </div>
              )}
              {diff.data.sql ? (
                <div className="space-y-2">
                  <p className="text-xs text-muted-foreground">
                    {t('schemaDiff.review')}
                  </p>
                  <Button variant="outline" size="sm" onClick={copySql}>
                    {t('schemaDiff.copySql')}
                  </Button>
                </div>
              ) : null}
            </>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
