import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useSetTolerations } from '../../queries/nodeMoves'
import {
  normalizeTolerations,
  rowsFromTolerations,
  sameJSON,
  tolerationsFromRows,
  type TaintRow,
} from '../../lib/taintRows'
import type { Placement } from '../../types/nodeMoves'
import { TaintRowsEditor } from './TaintRowsEditor'

// Editable tolerations plus a read-only affinity summary: affinity terms
// are structured, so they stay in app.yaml or the CLI.
export function TolerationsSection({
  name,
  placement,
}: {
  name: string
  placement: Placement
}) {
  const { t } = useTranslation('nodes')
  const saved = normalizeTolerations(placement.tolerations ?? [])
  const [rows, setRows] = useState<TaintRow[]>(() => rowsFromTolerations(saved))
  const save = useSetTolerations(name)
  const parsed = tolerationsFromRows(rows)
  const dirty = !sameJSON(parsed.tolerations, saved)
  const aff = placement.affinity
  const terms = [
    ...(aff?.node_required ?? []).map(
      (x) =>
        `${t('tolerations.nodeRequired')}: ${x.key} ${x.op} ${(x.values ?? []).join(',')}`,
    ),
    ...(aff?.node_preferred ?? []).map(
      (x) =>
        `${t('tolerations.nodePreferred')}: ${x.term.key} ${x.term.op} ${(x.term.values ?? []).join(',')} (${x.weight})`,
    ),
    ...(aff?.together ?? []).map(
      (x) =>
        `${t('tolerations.together')}: ${x.match.map((m) => `${m.key} ${m.op} ${(m.values ?? []).join(',')}`).join(', ')}`,
    ),
    ...(aff?.apart ?? []).map(
      (x) =>
        `${t('tolerations.apart')}: ${x.match.map((m) => `${m.key} ${m.op} ${(m.values ?? []).join(',')}`).join(', ')}`,
    ),
  ]

  return (
    <div className="space-y-2">
      <p className="text-sm font-medium text-foreground">
        {t('tolerations.title')}
      </p>
      <p className="text-xs text-muted-foreground">
        {rows.length === 0 ? t('tolerations.empty') : t('tolerations.hint')}
      </p>
      <TaintRowsEditor
        rows={rows}
        onChange={setRows}
        withOperator
        addLabel={t('tolerations.add')}
        disabled={save.isPending}
      />
      {parsed.error ? (
        <p className="text-sm text-destructive">{t('taints.incomplete')}</p>
      ) : null}
      {save.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button
        type="button"
        size="sm"
        disabled={save.isPending || !dirty || parsed.error !== null}
        onClick={() =>
          save.mutate(parsed.tolerations, {
            onSuccess: () =>
              toast.add({ title: t('tolerations.saved'), type: 'success' }),
          })
        }
      >
        {save.isPending ? t('taints.saving') : t('tolerations.save')}
      </Button>

      <p className="pt-2 text-sm font-medium text-foreground">
        {t('tolerations.affinityTitle')}
      </p>
      {terms.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('tolerations.affinityEmpty')}
        </p>
      ) : (
        <ul className="space-y-0.5 font-mono text-xs text-foreground">
          {terms.map((term) => (
            <li key={term}>{term}</li>
          ))}
        </ul>
      )}
      <p className="text-xs text-muted-foreground">
        {t('tolerations.affinityHint')}
      </p>
    </div>
  )
}
