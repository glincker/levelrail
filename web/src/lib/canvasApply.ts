import type { ConnectionStep } from './canvasDraft'

export interface StepResult {
  step: ConnectionStep
  status: 'applied' | 'failed' | 'skipped'
  error?: string
}

export interface StepRunner {
  connect: (app: string, database: string, envVar: string) => Promise<void>
  disconnect: (app: string, envVar: string) => Promise<void>
}

// Stops at the first failure and marks the rest skipped, like the plan apply does.
export async function runConnectionSteps(
  steps: readonly ConnectionStep[],
  runner: StepRunner,
): Promise<StepResult[]> {
  const out: StepResult[] = []
  let failed = false
  for (const step of steps) {
    if (failed) {
      out.push({ step, status: 'skipped' })
      continue
    }
    try {
      if (step.kind === 'connect') {
        await runner.connect(step.app, step.database, step.envVar)
      } else {
        await runner.disconnect(step.app, step.envVar)
      }
      out.push({ step, status: 'applied' })
    } catch (e) {
      failed = true
      out.push({
        step,
        status: 'failed',
        error: e instanceof Error ? e.message : String(e),
      })
    }
  }
  return out
}
