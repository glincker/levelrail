/** StepProps is what the wizard shell hands every step. */
export interface StepProps {
  onContinue: () => void
  onSkip: () => void
  pending: boolean
}

// Poll cadence and budgets for the live verification steps.
export const DNS_POLL_INTERVAL_MS = 5_000
export const CERT_POLL_INTERVAL_MS = 5_000
export const DOMAIN_POLL_BUDGET_MS = 15 * 60_000
export const GIT_POLL_INTERVAL_MS = 5_000
export const GIT_POLL_BUDGET_MS = 10 * 60_000
export const APP_POLL_INTERVAL_MS = 3_000
export const APP_POLL_BUDGET_MS = 10 * 60_000
export const APP_SLOW_AFTER_MS = 2 * 60_000

/** SAMPLE_APP is the one-click first deploy: small, public, and serves 200 on /. */
export const SAMPLE_APP = {
  name: 'sample-app',
  image: 'nginx:alpine',
  port: 80,
  readinessPath: '/',
} as const
