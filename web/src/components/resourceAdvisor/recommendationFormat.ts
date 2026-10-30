import { formatUsd } from '../../lib/assetsFormat'

// Null hides the saving line when the engine priced no drop.
export function formatMonthlySaving(usd?: number): string | null {
  if (usd === undefined || !Number.isFinite(usd) || usd <= 0) return null
  return formatUsd(usd)
}
