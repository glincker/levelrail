import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useNodeTaints, useSetNodeTaints } from '../queries/nodeMoves'
import {
  normalizeTaints,
  rowsFromTaints,
  sameJSON,
  taintsFromRows,
  type TaintRow,
} from '../lib/taintRows'
import type { NodeTaint } from '../types/nodeMoves'
import { TaintRowsEditor } from './nodes/TaintRowsEditor'

export function NodeTaintsCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const taints = useNodeTaints(nodeId)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('taints.title')}</CardTitle>
        <CardDescription>{t('taints.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {taints.isPending ? (
          <Skeleton className="h-9 w-full" />
        ) : taints.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="space-y-2">
              <p>
                {t('taints.loadFailed')} {taints.error.message}
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void taints.refetch()}
              >
                {t('taints.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <TaintsForm
            key={JSON.stringify(taints.data)}
            nodeId={nodeId}
            saved={normalizeTaints(taints.data)}
          />
        )}
      </CardContent>
    </Card>
  )
}

function TaintsForm({ nodeId, saved }: { nodeId: string; saved: NodeTaint[] }) {
  const { t } = useTranslation('nodes')
  const [rows, setRows] = useState<TaintRow[]>(() => rowsFromTaints(saved))
  const save = useSetNodeTaints(nodeId)
  const parsed = taintsFromRows(rows)
  const dirty = !sameJSON(parsed.taints, saved)

  return (
    <>
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('taints.empty')}</p>
      ) : null}
      <TaintRowsEditor
        rows={rows}
        onChange={setRows}
        addLabel={t('taints.add')}
        disabled={save.isPending}
      />
      <p className="text-xs text-muted-foreground">{t('taints.effectHint')}</p>

      {parsed.error ? (
        <p className="text-sm text-destructive">
          {parsed.error === 'duplicate'
            ? t('taints.duplicate')
            : t('taints.incomplete')}
        </p>
      ) : null}

      {save.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          disabled={save.isPending || !dirty || parsed.error !== null}
          onClick={() =>
            save.mutate(parsed.taints, {
              onSuccess: () =>
                toast.add({ title: t('taints.saved'), type: 'success' }),
            })
          }
        >
          {save.isPending ? t('taints.saving') : t('taints.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={save.isPending || !dirty}
          onClick={() => {
            setRows(rowsFromTaints(saved))
            save.reset()
          }}
        >
          {t('taints.reset')}
        </Button>
      </div>
    </>
  )
}
