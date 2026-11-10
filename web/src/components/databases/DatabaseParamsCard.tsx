import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  buildChange,
  databaseParamsQuery,
  useDatabaseParamsAction,
  type ParamsAction,
} from '../../queries/databaseParams'

export function DatabaseParamsCard({ database }: { database: string }) {
  const { t } = useTranslation('databases')
  const { data } = useQuery(databaseParamsQuery(database))
  const action = useDatabaseParamsAction(database)
  const [edits, setEdits] = useState<Record<string, string>>({})
  const [confirming, setConfirming] = useState(false)

  if (!data?.supported) return null

  const change = buildChange(data.params, edits)
  const dirty =
    Object.keys(change.set ?? {}).length > 0 || (change.unset ?? []).length > 0

  function run(a: ParamsAction, done?: () => void) {
    action.mutate(a, {
      onSuccess: () => {
        setEdits({})
        setConfirming(false)
        done?.()
      },
      onError: (error) =>
        toast.add({
          title: t('params.failed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>{t('params.title')}</CardTitle>
        {data.has_pending ? (
          <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-xs text-amber-700 dark:text-amber-300">
            {t('params.staged')}
          </span>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('params.description')}
        </p>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted-foreground">
              <tr>
                <th className="py-1 pr-3 font-medium">{t('params.name')}</th>
                <th className="py-1 pr-3 font-medium">{t('params.default')}</th>
                <th className="py-1 pr-3 font-medium">{t('params.current')}</th>
                <th className="py-1 font-medium">{t('params.next')}</th>
              </tr>
            </thead>
            <tbody>
              {data.params.map((p) => {
                const value = edits[p.name] ?? p.pending ?? p.current
                const listId = p.options ? `param-opts-${p.name}` : undefined
                return (
                  <tr key={p.name} className="border-t border-border">
                    <td className="py-1.5 pr-3">
                      <code className="font-mono text-xs">{p.name}</code>
                      <div className="text-xs text-muted-foreground">
                        {p.description}
                      </div>
                    </td>
                    <td className="py-1.5 pr-3 font-mono text-xs">
                      {p.default || t('params.engineDefault')}
                    </td>
                    <td className="py-1.5 pr-3 font-mono text-xs">
                      {p.current || t('params.engineDefault')}
                    </td>
                    <td className="py-1.5">
                      <Input
                        aria-label={p.name}
                        className="h-8 w-36 font-mono text-xs"
                        list={listId}
                        placeholder={p.default}
                        value={value}
                        onChange={(e) =>
                          setEdits((prev) => ({
                            ...prev,
                            [p.name]: e.target.value,
                          }))
                        }
                      />
                      {p.options ? (
                        <datalist id={listId}>
                          {p.options.map((o) => (
                            <option key={o} value={o} />
                          ))}
                        </datalist>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            size="sm"
            disabled={!dirty || action.isPending}
            onClick={() => run({ kind: 'stage', change })}
          >
            {t('params.stage')}
          </Button>
          {data.has_pending ? (
            <>
              {confirming ? (
                <Button
                  type="button"
                  size="sm"
                  variant="destructive"
                  disabled={action.isPending}
                  onClick={() => run({ kind: 'apply' })}
                >
                  {t('params.confirmApply')}
                </Button>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={action.isPending}
                  onClick={() => setConfirming(true)}
                >
                  {t('params.apply')}
                </Button>
              )}
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={action.isPending}
                onClick={() => run({ kind: 'discard' })}
              >
                {t('params.discard')}
              </Button>
            </>
          ) : null}
        </div>
        <p className="text-xs text-muted-foreground">
          {data.has_pending ? t('params.pendingNote') : t('params.note')}
        </p>
      </CardContent>
    </Card>
  )
}
