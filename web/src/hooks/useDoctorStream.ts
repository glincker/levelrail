import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  reduceDoctor,
  startDoctorState,
  type DoctorState,
} from '../lib/doctorModel'
import { fixKeys } from '../queries/fixit'
import {
  runDoctorCheck,
  systemDoctorKeys,
  type DoctorStreamEvent,
} from '../queries/systemDoctor'

const STREAM_URL = '/api/v1/system/doctor/stream'

export interface DoctorStream {
  state: DoctorState
  /** Runs every check again from scratch. */
  rerun: () => void
  /** Re-runs one registered check and swaps its results in place. */
  recheck: (checkId: string) => Promise<void>
}

/** useDoctorStream paints each check as the server finishes it. */
export function useDoctorStream(): DoctorStream {
  const qc = useQueryClient()
  const [state, setState] = useState<DoctorState>(startDoctorState)
  const source = useRef<EventSource | null>(null)

  const open = useCallback(() => {
    source.current?.close()
    const es = new EventSource(STREAM_URL)
    source.current = es
    es.onmessage = (m: MessageEvent<string>) => {
      const ev = JSON.parse(m.data) as DoctorStreamEvent
      setState((s) => reduceDoctor(s, ev))
      if (ev.type === 'done') {
        es.close()
        void qc.invalidateQueries({ queryKey: systemDoctorKeys.all })
        void qc.invalidateQueries({ queryKey: fixKeys.all })
      }
    }
    es.onerror = () => {
      es.close()
      setState((s) => ({ ...s, running: false, error: 'stream' }))
    }
  }, [qc])

  const rerun = useCallback(() => {
    setState(startDoctorState())
    open()
  }, [open])

  useEffect(() => {
    open()
    return () => {
      source.current?.close()
    }
  }, [open])

  const recheck = useCallback(
    async (checkId: string) => {
      const checks = await runDoctorCheck(checkId)
      setState((s) => ({
        ...s,
        byCheck: { ...s.byCheck, [checkId]: checks },
      }))
      void qc.invalidateQueries({ queryKey: fixKeys.all })
    },
    [qc],
  )

  return { state, rerun, recheck }
}
