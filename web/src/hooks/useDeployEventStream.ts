import { useEffect, useReducer, useState } from 'react'
import {
  applyMessage,
  EMPTY_EVENT_STATE,
  parseEventMessage,
  type DeployEventState,
} from '../lib/deployEvents'
import type { DeployEventMessage } from '../types/deployEvents'

export type DeployEventConnection = 'connecting' | 'live' | 'ended' | 'error'

interface Slot {
  id: string
  state: DeployEventState
}

type Action = { id: string; msg: DeployEventMessage } | { reset: string }

function reduce(slot: Slot, action: Action): Slot {
  if ('reset' in action) return { id: action.reset, state: EMPTY_EVENT_STATE }
  if (action.id !== slot.id) return slot
  return { id: slot.id, state: applyMessage(slot.state, action.msg) }
}

/**
 * Follows GET /api/v1/deployments/{id}/timeline/stream. The server sends one
 * snapshot, then changed events, then an end message after which the hook
 * closes the source so the browser does not reconnect.
 */
export function useDeployEventStream(id: string): {
  state: DeployEventState
  connection: DeployEventConnection
} {
  const [slot, dispatch] = useReducer(reduce, {
    id,
    state: EMPTY_EVENT_STATE,
  })
  const [connection, setConnection] =
    useState<DeployEventConnection>('connecting')
  const [seen, setSeen] = useState(id)
  if (seen !== id) {
    setSeen(id)
    setConnection('connecting')
    dispatch({ reset: id })
  }

  useEffect(() => {
    const source = new EventSource(
      `/api/v1/deployments/${encodeURIComponent(id)}/timeline/stream`,
    )
    source.onopen = () => {
      setConnection('live')
    }
    source.onerror = () => {
      setConnection('error')
    }
    source.onmessage = (ev: MessageEvent<string>) => {
      const msg = parseEventMessage(ev.data)
      if (!msg) return
      dispatch({ id, msg })
      if (msg.type === 'end') {
        setConnection('ended')
        source.close()
      }
    }
    return () => {
      source.close()
    }
  }, [id])

  return { state: slot.id === id ? slot.state : EMPTY_EVENT_STATE, connection }
}
