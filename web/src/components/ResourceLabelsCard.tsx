import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TagIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import {
  useResourceLabels,
  useSetResourceLabels,
  type LabelKindPath,
} from '../queries/resourceLabels'
import {
  labelsFromRows,
  rowsFromLabels,
  sameLabels,
  type LabelRow,
} from '../lib/labelRows'
import { LabelRowsEditor } from './nodes/LabelRowsEditor'

export function ResourceLabelsCard({
  path,
  name,
}: {
  path: LabelKindPath
  name: string
}) {
  const { t } = useTranslation('common')
  const labels = useResourceLabels(path, name)
  const save = useSetResourceLabels(path, name)
  const [rows, setRows] = useState<LabelRow[] | null>(null)
  const saved = labels.data ?? {}
  const parsed = rows ? labelsFromRows(rows) : null
  const dirty = parsed ? !sameLabels(parsed.labels, saved) : false
  const keys = Object.keys(saved).sort()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('resourceLabels.title')}</CardTitle>
        <CardDescription>{t('resourceLabels.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {rows === null ? (
          <div className="flex flex-wrap items-center gap-2">
            {keys.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('resourceLabels.empty')}
              </p>
            ) : (
              keys.map((key) => (
                <Badge key={key} variant="muted" className="font-mono">
                  <TagIcon aria-hidden="true" />
                  {key}={saved[key]}
                </Badge>
              ))
            )}
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={labels.isPending}
              onClick={() => setRows(rowsFromLabels(saved))}
            >
              {t('resourceLabels.edit')}
            </Button>
          </div>
        ) : (
          <>
            <LabelRowsEditor
              rows={rows}
              onChange={setRows}
              keyPlaceholder={t('resourceLabels.keyPlaceholder')}
              valuePlaceholder={t('resourceLabels.valuePlaceholder')}
              addLabel={t('resourceLabels.add')}
              removeLabel={(key) => t('resourceLabels.remove', { key })}
              disabled={save.isPending}
            />
            {parsed?.error ? (
              <p className="text-sm text-destructive">
                {parsed.error === 'duplicate'
                  ? t('resourceLabels.duplicate')
                  : t('resourceLabels.incomplete')}
              </p>
            ) : null}
            {save.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertDescription>{save.error.message}</AlertDescription>
              </Alert>
            ) : null}
            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="sm"
                disabled={save.isPending || !dirty || parsed?.error !== null}
                onClick={() =>
                  parsed &&
                  save.mutate(parsed.labels, {
                    onSuccess: () => {
                      toast.add({
                        title: t('resourceLabels.saved'),
                        type: 'success',
                      })
                      setRows(null)
                    },
                  })
                }
              >
                {save.isPending
                  ? t('resourceLabels.saving')
                  : t('resourceLabels.save')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={save.isPending}
                onClick={() => {
                  setRows(null)
                  save.reset()
                }}
              >
                {t('resourceLabels.cancel')}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
