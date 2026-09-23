import { useEffect, useState } from 'react'

// SSE hook for GET /api/v1/apps/{name}/deploys/{deployId}/steps
// (internal/api/deploy_steps.go): discrete, named pipeline-phase
// transitions (detecting/building/pushing/deploying, or a terminal
// failed), the counterpart to useLogStream for a checklist-style
// progress view rather than a scrolling log. Deliberately its own hook,
// not a generalization of useLogStream: the wire shape is different
// (named steps with a status, not stdout/stderr lines) and the update
// rate is much lower (a handful of events per attempt, not one per
// output line), so a ring buffer and reconnect-duplicate suppression
// (useLogStream's own concerns) don't apply the same way here: a step
// event is idempotent by (step, status), so a duplicate on reconnect is
// harmless (see mergeStep below) rather than something to suppress.
//
//   Content-Type: text/event-stream
//   data: { "step": string, "status": "running" | "done" | "failed", "timestamp": string }

export type DeployStepStatus = 'running' | 'done' | 'failed'

export interface DeployStepEvent {
  step: string
  status: DeployStepStatus
  timestamp: string
}

export type DeployStepStreamConnectionState = 'connecting' | 'open' | 'error'

export interface UseDeployStepStreamResult {
  /** Every step transition seen so far, in the order the server sent
   *  them (duplicates on reconnect keep their original position, see
   *  mergeStep). */
  steps: DeployStepEvent[]
  connectionState: DeployStepStreamConnectionState
}

interface RawStepEventPayload {
  step?: unknown
  status?: unknown
  timestamp?: unknown
}

function isDeployStepStatus(value: unknown): value is DeployStepStatus {
  return value === 'running' || value === 'done' || value === 'failed'
}

function parseStepEvent(raw: string): DeployStepEvent | null {
  try {
    const parsed = JSON.parse(raw) as RawStepEventPayload
    if (
      typeof parsed.step === 'string' &&
      isDeployStepStatus(parsed.status) &&
      typeof parsed.timestamp === 'string'
    ) {
      return {
        step: parsed.step,
        status: parsed.status,
        timestamp: parsed.timestamp,
      }
    }
  } catch {
    // Malformed payload: dropped, same tolerance useLogStream's own
    // parseEventPayload gives a non-JSON line.
  }
  return null
}

// mergeStep upserts ev by step name: a later event for the same step
// (e.g. "building"/running then "building"/done) replaces the earlier
// one in place rather than appending a second row, so a reconnect
// replaying the snapshot burst never duplicates a row in the rendered
// list.
function mergeStep(
  steps: DeployStepEvent[],
  ev: DeployStepEvent,
): DeployStepEvent[] {
  const index = steps.findIndex((s) => s.step === ev.step)
  if (index === -1) return [...steps, ev]
  const next = [...steps]
  next[index] = ev
  return next
}

export function useDeployStepStream(url: string): UseDeployStepStreamResult {
  const [steps, setSteps] = useState<DeployStepEvent[]>([])
  const [connectionState, setConnectionState] =
    useState<DeployStepStreamConnectionState>('connecting')
  const [urlForReset, setUrlForReset] = useState(url)
  if (url !== urlForReset) {
    setUrlForReset(url)
    setSteps([])
    setConnectionState('connecting')
  }

  useEffect(() => {
    const source = new EventSource(url)

    source.onopen = () => {
      setConnectionState('open')
    }
    source.onerror = () => {
      setConnectionState('error')
    }
    source.onmessage = (event: MessageEvent<string>) => {
      const parsed = parseStepEvent(event.data)
      if (!parsed) return
      setSteps((prev) => mergeStep(prev, parsed))
    }

    return () => {
      source.close()
    }
  }, [url])

  return { steps, connectionState }
}
