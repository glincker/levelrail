import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  type NodeThresholds,
  type NodeThresholdValues,
  useNodeThresholds,
  useSetNodeThresholds,
} from '../../queries/nodeThresholds'

type ThresholdKey = keyof NodeThresholdValues
type Draft = Record<ThresholdKey, string>

const KEYS: ThresholdKey[] = ['cpu_percent', 'memory_percent', 'disk_percent']

function draftOf(t: NodeThresholds): Draft {
  const s = (v: number | null) => (v === null ? '' : String(v))
  return {
    cpu_percent: s(t.override.cpu_percent),
    memory_percent: s(t.override.memory_percent),
    disk_percent: s(t.override.disk_percent),
  }
}

function toOverride(d: Draft): NodeThresholdValues {
  const n = (v: string) => (v.trim() === '' ? null : Number(v))
  return {
    cpu_percent: n(d.cpu_percent),
    memory_percent: n(d.memory_percent),
    disk_percent: n(d.disk_percent),
  }
}

// Draft starts from the stored override and the placeholder shows the live default, so a blank field means "use default".
function ThresholdsForm({
  nodeId,
  data,
}: {
  nodeId: string
  data: NodeThresholds
}) {
  const { t } = useTranslation('nodes')
  const save = useSetNodeThresholds(nodeId)
  const [draft, setDraft] = useState<Draft>(() => draftOf(data))
  const saved = draftOf(data)
  const unchanged = KEYS.every((k) => draft[k] === saved[k])
  const invalid = KEYS.some((k) => {
    if (draft[k].trim() === '') return false
    const v = Number(draft[k])
    return !Number.isInteger(v) || v < 1 || v > 100
  })
  const submit = (next: Draft) => {
    save.mutate(toOverride(next), {
      onSuccess: (updated) => {
        setDraft(draftOf(updated))
        toast.add({ title: t('thresholds.saved'), type: 'success' })
      },
    })
  }

  return (
    <CardContent className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-3">
        {KEYS.map((k) => (
          <Field key={k}>
            <FieldLabel htmlFor={`node-threshold-${k}`}>
              {t(`thresholds.${k}`)}
            </FieldLabel>
            <Input
              id={`node-threshold-${k}`}
              inputMode="numeric"
              value={draft[k]}
              placeholder={t('thresholds.defaultPlaceholder', {
                value: data.defaults[k] ?? '',
              })}
              disabled={save.isPending}
              onChange={(e) => setDraft({ ...draft, [k]: e.target.value })}
            />
            <FieldDescription>
              {t('thresholds.effective', { value: data.effective[k] ?? '' })}
            </FieldDescription>
          </Field>
        ))}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={save.isPending || unchanged || invalid}
          onClick={() => submit(draft)}
        >
          {t('thresholds.save')}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={save.isPending || KEYS.every((k) => saved[k] === '')}
          onClick={() =>
            submit({ cpu_percent: '', memory_percent: '', disk_percent: '' })
          }
        >
          {t('thresholds.reset')}
        </Button>
      </div>
      <FieldDescription>
        {invalid
          ? t('thresholds.invalid')
          : t('thresholds.description', { count: data.sustained_intervals })}
      </FieldDescription>
      {save.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </CardContent>
  )
}

export function NodeThresholdsCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const q = useNodeThresholds(nodeId)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('thresholds.title')}</CardTitle>
      </CardHeader>
      {q.isPending ? (
        <CardContent>
          <p className="text-sm text-muted-foreground">
            {t('thresholds.loading')}
          </p>
        </CardContent>
      ) : q.isError ? (
        <CardContent>
          <Alert variant="destructive">
            <AlertDescription>{q.error.message}</AlertDescription>
          </Alert>
        </CardContent>
      ) : (
        <ThresholdsForm key={nodeId} nodeId={nodeId} data={q.data} />
      )}
    </Card>
  )
}
