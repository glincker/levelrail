export const ONBOARDING_STEPS = [
  'welcome',
  'profile',
  'preferences',
  'finish',
] as const

export type OnboardingStepId = (typeof ONBOARDING_STEPS)[number]

/** stepIndex maps the server's current_step to a position; unknown ids start over at the welcome step. */
export function stepIndex(current: string | undefined): number {
  const at = ONBOARDING_STEPS.findIndex((s) => s === current)
  return at === -1 ? 0 : at
}
