import { useNodeListOptional } from '../../queries/nodes'
import { useFleetResourceUsage } from '../../queries/fleetResourceUsage'
import { toPlacementNode, type PlacementNode } from './placementPick'

export function usePlacementNodes(): readonly PlacementNode[] {
  const list = useNodeListOptional().data
  const usage = useFleetResourceUsage().data?.nodes
  return (list ?? []).map((n) =>
    toPlacementNode(
      n,
      usage?.find((u) => u.node_id === n.id),
    ),
  )
}
