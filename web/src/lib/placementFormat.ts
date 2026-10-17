import { formatCores, formatSize } from './format'
import type { PlacementDimension } from '../types/placementDecision'

// cpu is millicores, memory and disk are bytes.
export function formatDimension(d: PlacementDimension, v: number): string {
  return d.name === 'cpu' ? `${formatCores(v / 1000)} CPU` : formatSize(v)
}
