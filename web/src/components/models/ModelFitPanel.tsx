import { useMemo } from 'react'
import { SkeletonLine } from '@/components/kit'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import type { DeployFormState } from '../../lib/modelDeployForm'
import { buildFitRequest, isSelectedNode } from '../../lib/modelFit'
import { useFitCheck } from '../../queries/modelFit'
import { FitNodeList } from './ModelFitView'

const DEBOUNCE_MS = 400

// Per-node VRAM fit for the deploy dialog, updated as the form changes.
export function ModelFitPanel({ form }: { form: DeployFormState }) {
  const debounced = useDebouncedValue(form, DEBOUNCE_MS)
  const req = useMemo(() => buildFitRequest(debounced), [debounced])
  const { data, isPending, error } = useFitCheck(req)
  if (!req) return null
  return (
    <section
      aria-label="VRAM fit"
      className="space-y-1.5 rounded-md border border-border p-3"
    >
      <h4 className="text-xs font-medium text-foreground">VRAM fit per node</h4>
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error.message}
        </p>
      ) : isPending ? (
        <SkeletonLine />
      ) : (
        <FitNodeList
          report={data}
          highlight={(n) => isSelectedNode(n, form.node)}
        />
      )}
    </section>
  )
}
