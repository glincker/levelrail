import { useMemo, useState } from 'react'
import { useQueries, useQueryClient } from '@tanstack/react-query'
import {
  useApplyIac,
  usePlanIac,
  type IacApplyResult,
  type IacPlan,
} from '../queries/iac'
import {
  appConnectionKeys,
  appConnectionsQueryOptions,
  createAppConnection,
  deleteAppConnection,
  type AppConnection,
} from '../queries/appConnections'
import { projectTopologyKeys } from '../queries/projectTopology'
import {
  disconnectApps,
  draftFiles,
  hasPlanOps,
  resolveConnectionSteps,
  type DraftOp,
} from '../lib/canvasDraft'
import { runConnectionSteps, type StepResult } from '../lib/canvasApply'

export interface CanvasOutcome {
  iac: IacApplyResult | null
  steps: StepResult[]
  ok: boolean
}

// Owns the review and apply round trips. Nothing here runs until the caller
// invokes review() or apply(), so the draft stays client-side until then.
export function useCanvasReview(
  projectId: string,
  projectName: string,
  ops: readonly DraftOp[],
) {
  const queryClient = useQueryClient()
  const planIac = usePlanIac()
  const applyIac = useApplyIac()
  const [open, setOpen] = useState(false)
  const [plan, setPlan] = useState<IacPlan | null>(null)
  const [outcome, setOutcome] = useState<CanvasOutcome | null>(null)
  const [applying, setApplying] = useState(false)

  const apps = useMemo(() => disconnectApps(ops), [ops])
  const lookups = useQueries({
    queries: apps.map((app) => ({
      ...appConnectionsQueryOptions(app),
      enabled: open,
    })),
  })
  const lookupsLoading = lookups.some((q) => q.isLoading)
  const resolved = useMemo(() => {
    const live: Record<string, readonly AppConnection[]> = {}
    apps.forEach((app, i) => {
      live[app] = lookups[i]?.data ?? []
    })
    return resolveConnectionSteps(ops, live)
  }, [apps, lookups, ops])

  const files = useMemo(() => draftFiles(ops, projectName), [ops, projectName])

  function review() {
    setOpen(true)
    setOutcome(null)
    setPlan(null)
    planIac.reset()
    if (hasPlanOps(ops)) {
      planIac.mutate({ files }, { onSuccess: setPlan })
    }
  }

  function close() {
    setOpen(false)
    setOutcome(null)
    planIac.reset()
  }

  async function apply(): Promise<CanvasOutcome> {
    setApplying(true)
    try {
      let iac: IacApplyResult | null = null
      if (hasPlanOps(ops)) {
        iac = await applyIac.mutateAsync({
          files,
          expected_plan_hash: plan?.hash,
        })
      }
      const iacClean = !iac || (iac.failed === 0 && iac.skipped === 0)
      const steps = iacClean
        ? await runConnectionSteps(resolved.steps, {
            connect: async (app, database, envVar) => {
              await createAppConnection(app, {
                database,
                envVar: envVar || undefined,
              })
            },
            disconnect: (app, envVar) => deleteAppConnection(app, envVar),
          })
        : resolved.steps.map((step) => ({ step, status: 'skipped' as const }))
      const result: CanvasOutcome = {
        iac,
        steps,
        ok: iacClean && steps.every((s) => s.status === 'applied'),
      }
      setOutcome(result)
      return result
    } finally {
      setApplying(false)
      void queryClient.invalidateQueries({
        queryKey: projectTopologyKeys.detail(projectId),
      })
      void queryClient.invalidateQueries({ queryKey: appConnectionKeys.all })
    }
  }

  return {
    open,
    plan,
    planError: planIac.error,
    planning: planIac.isPending,
    applyError: applyIac.error,
    applying,
    outcome,
    resolved,
    lookupsLoading,
    review,
    close,
    apply,
  }
}
