import { useTranslation } from 'react-i18next'
import { useNodeListOptional } from '../../queries/nodes'

// Resolves a node id to its display name, falling back to the raw id and
// naming the control plane for an empty id.
export function useNodeNames(): (id: string | undefined) => string {
  const { t } = useTranslation('nodes')
  const { data } = useNodeListOptional()
  const names = new Map((data ?? []).map((n) => [n.id, n.name]))
  return (id) => {
    if (!id) {
      return t('placement.controlPlane')
    }
    return names.get(id) ?? id
  }
}
