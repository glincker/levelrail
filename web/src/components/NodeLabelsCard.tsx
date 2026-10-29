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
import { useNodeLabels, useSetNodeLabels } from '../queries/nodeMoves'
import {
  labelsFromRows,
  newLabelRow,
  rowsFromLabels,
  sameLabels,
  type LabelRow,
} from '../lib/labelRows'
import type { NodeLabels } from '../types/nodeMoves'
import { LabelRowsEditor } from './nodes/LabelRowsEditor'

const KNOWN_KEYS = ['region', 'zone', 'provider'] as const

export function NodeLabelsCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const labels = useNodeLabels(nodeId)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('labels.title')}</CardTitle>
        <CardDescription>{t('labels.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {labels.isPending ? (
          <Skeleton className="h-9 w-full" />
        ) : labels.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription className="space-y-2">
              <p>
                {t('labels.loadFailed')} {labels.error.message}
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void labels.refetch()}
              >
                {t('labels.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <LabelsForm
            key={JSON.stringify(labels.data)}
            nodeId={nodeId}
            saved={labels.data}
          />
        )}
      </CardContent>
    </Card>
  )
}

function LabelsForm({ nodeId, saved }: { nodeId: string; saved: NodeLabels }) {
  const { t } = useTranslation('nodes')
  const [rows, setRows] = useState<LabelRow[]>(() => rowsFromLabels(saved))
  const save = useSetNodeLabels(nodeId)
  const parsed = labelsFromRows(rows)
  const dirty = !sameLabels(parsed.labels, saved)

  return (
    <>
      {rows.length === 0 ? (
        <div className="text-sm">
          <p className="text-foreground">{t('labels.empty')}</p>
          <p className="text-muted-foreground">{t('labels.emptyHint')}</p>
        </div>
      ) : null}

      <LabelRowsEditor
        rows={rows}
        onChange={setRows}
        keyPlaceholder={t('labels.keyPlaceholder')}
        valuePlaceholder={t('labels.valuePlaceholder')}
        addLabel={t('labels.add')}
        removeLabel={(key) => t('labels.remove', { key })}
        disabled={save.isPending}
      />

      <div className="flex flex-wrap items-center gap-2">
        {KNOWN_KEYS.filter((key) => !rows.some((r) => r.key === key)).map(
          (key) => (
            <Button
              key={key}
              type="button"
              size="sm"
              variant="outline"
              disabled={save.isPending}
              onClick={() => setRows([...rows, newLabelRow(key, '')])}
            >
              {t('labels.addKnown', { key })}
            </Button>
          ),
        )}
      </div>
      <p className="text-xs text-muted-foreground">{t('labels.knownHint')}</p>

      <p className="text-xs text-muted-foreground">
        {t('labels.keyRule')} {t('labels.valueRule')}
      </p>

      {parsed.error ? (
        <p className="text-sm text-destructive">
          {parsed.error === 'duplicate'
            ? t('labels.duplicate')
            : t('labels.incomplete')}
        </p>
      ) : null}

      {save.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          disabled={save.isPending || !dirty || parsed.error !== null}
          onClick={() =>
            save.mutate(parsed.labels, {
              onSuccess: () =>
                toast.add({ title: t('labels.saved'), type: 'success' }),
            })
          }
        >
          {save.isPending ? t('labels.saving') : t('labels.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={save.isPending || !dirty}
          onClick={() => {
            setRows(rowsFromLabels(saved))
            save.reset()
          }}
        >
          {t('labels.reset')}
        </Button>
      </div>
    </>
  )
}
