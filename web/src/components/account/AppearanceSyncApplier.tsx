import { useAppearanceSyncRunner } from '../../queries/appearanceSync'

// Renders nothing: keeps appearance in step with the account while signed in.
export function AppearanceSyncApplier() {
  useAppearanceSyncRunner()
  return null
}
