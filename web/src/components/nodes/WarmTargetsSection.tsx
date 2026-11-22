import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useAppWarm,
  useSetAppWarm,
  type WarmTargets,
} from '../../queries/appWarm'
import { useNodeListOptional } from '../../queries/nodes'
import {
  labelsFromRows,
  rowsFromLabels,
  sameLabels,
  type LabelRow,
} from '../../lib/labelRows'
import { LabelRowsEditor } from './LabelRowsEditor'

// Keeps the app's image pulled on chosen nodes or a label group so a move or
// failover starts without a cold pull. Off means nothing is kept warm.
export function WarmTargetsSection({ name }: { name: string }) {
  const warm = useAppWarm(name)
  if (warm.isPending) {
    return <Skeleton className="h-10 w-full" />
  }
  if (warm.isError) {
    return null
  }
  return (
    <WarmBody key={JSON.stringify(warm.data)} name={name} saved={warm.data} />
  )
}

function WarmBody({ name, saved }: { name: string; saved: WarmTargets }) {
  const { t } = useTranslation('nodes')
  const { data: nodeList } = useNodeListOptional()
  const save = useSetAppWarm(name)
  const hasSaved =
    saved.nodes.length > 0 || Object.keys(saved.labels).length > 0
  const [on, setOn] = useState(hasSaved)
  const [picked, setPicked] = useState<string[]>(saved.nodes)
  const [rows, setRows] = useState<LabelRow[]>(() =>
    rowsFromLabels(saved.labels),
  )
  const parsed = labelsFromRows(rows)
  const dirty =
    picked.length !== saved.nodes.length ||
    picked.some((id) => !saved.nodes.includes(id)) ||
    !sameLabels(parsed.labels, saved.labels)
  const empty = picked.length === 0 && Object.keys(parsed.labels).length === 0

  function submit(next: WarmTargets, done: string) {
    save.mutate(next, {
      onSuccess: () => toast.add({ title: done, type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('placement.warm.saveFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  function toggle(next: boolean) {
    setOn(next)
    if (!next && hasSaved) {
      setPicked([])
      setRows([])
      submit({ nodes: [], labels: {} }, t('placement.warm.cleared'))
    }
  }

  function pick(id: string) {
    setPicked((cur) =>
      cur.includes(id) ? cur.filter((n) => n !== id) : [...cur, id],
    )
  }

  const nodes = nodeList ?? []
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-4">
        <div className="space-y-0.5">
          <p className="text-sm font-medium text-foreground">
            {t('placement.warm.title')}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('placement.warm.hint')}
          </p>
        </div>
        <Switch
          checked={on}
          onCheckedChange={toggle}
          disabled={save.isPending}
          aria-label={t('placement.warm.switchAria')}
        />
      </div>
      {on ? (
        <div className="space-y-2">
          {nodes.length > 0 ? (
            <div
              className="flex flex-wrap gap-1.5"
              role="group"
              aria-label={t('placement.warm.nodesLabel')}
            >
              {nodes.map((n) => (
                <button
                  key={n.id}
                  type="button"
                  aria-pressed={picked.includes(n.id)}
                  disabled={save.isPending}
                  onClick={() => pick(n.id)}
                  className="rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                  <Badge
                    variant={picked.includes(n.id) ? 'success' : 'outline'}
                  >
                    {n.name}
                  </Badge>
                </button>
              ))}
            </div>
          ) : null}
          <LabelRowsEditor
            rows={rows}
            onChange={setRows}
            keyPlaceholder={t('placement.keyPlaceholder')}
            valuePlaceholder={t('placement.valuePlaceholder')}
            addLabel={t('placement.warm.addLabel')}
            removeLabel={(key) => t('placement.removeRow', { key })}
            disabled={save.isPending}
          />
          {parsed.error ? (
            <p className="text-sm text-destructive" role="alert">
              {parsed.error === 'duplicate'
                ? t('placement.duplicate')
                : t('placement.incomplete')}
            </p>
          ) : null}
          <Button
            type="button"
            size="sm"
            disabled={
              save.isPending || !dirty || parsed.error !== null || empty
            }
            onClick={() =>
              submit(
                { nodes: picked, labels: parsed.labels },
                t('placement.warm.saved'),
              )
            }
          >
            {save.isPending ? t('placement.saving') : t('placement.warm.save')}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
