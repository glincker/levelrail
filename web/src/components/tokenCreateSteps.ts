import type { CreateFlowStep } from './CreateFlowKit'

// Shared between CreateTokenDialog (step 1, the form) and
// TokenCreatedView (step 2, the one-time secret reveal) so both views'
// CreateFlowSteps indicator stays in sync. A plain data module, not a
// component, so react-refresh doesn't flag either consumer for
// co-exporting a constant alongside its component.
export const TOKEN_STEPS: CreateFlowStep[] = [
  { id: 'details', label: 'Details' },
  { id: 'reveal', label: 'Save your token' },
]
