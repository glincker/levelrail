import { useTranslation } from 'react-i18next'
import type { MessageRef } from '../../lib/deployState'

type LooseT = (key: string, values?: Record<string, string | number>) => string

// State keys are computed in lib/deployState.ts, so they cannot be checked
// against the typed resource map at the call site.
export function useDeployDetailT(): {
  t: LooseT
  msg: (ref: MessageRef) => string
} {
  const { t } = useTranslation('deployDetail')
  const loose = t as unknown as LooseT
  return { t: loose, msg: (ref) => loose(ref.key, ref.values) }
}
