import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { measurableTargets } from '../../lib/cleanup'
import { usePreviewCleanup } from '../../queries/cleanup'
import type { CleanupTarget } from '../../types/cleanup'

export interface Reclaimable {
  rows: number
  bytes: number
}

export interface Calculation {
  running: boolean
  done: number
  total: number
  error: string | null
}

const IDLE: Calculation = { running: false, done: 0, total: 0, error: null }

/** Sums a preview of every enabled target, one after another, only when asked. */
export function useReclaimable(targets: CleanupTarget[]) {
  const { t } = useTranslation('cleanup')
  const preview = usePreviewCleanup()
  const [sum, setSum] = useState<Reclaimable | null>(null)
  const [state, setState] = useState<Calculation>(IDLE)

  async function calculate() {
    const list = measurableTargets(targets)
    setState({ running: true, done: 0, total: list.length, error: null })
    const next: Reclaimable = { rows: 0, bytes: 0 }
    for (const [i, target] of list.entries()) {
      try {
        const r = await preview.mutateAsync({
          id: target.id,
          days: target.days,
        })
        next.rows += r.rows
        next.bytes += r.bytes
      } catch (err) {
        setSum(null)
        setState({
          running: false,
          done: i,
          total: list.length,
          error: t('hero.calculateFailed', {
            title: target.title,
            message: err instanceof Error ? err.message : String(err),
          }),
        })
        return
      }
      setState({ running: true, done: i + 1, total: list.length, error: null })
    }
    setSum(next)
    setState(IDLE)
  }

  return {
    reclaimable: sum,
    calculation: state,
    canCalculate: measurableTargets(targets).length > 0,
    calculate: () => {
      void calculate()
    },
  }
}
