import type { TemplateRequirements } from '../queries/serviceTemplates'

/** Elevated templates need the operator's explicit acknowledgement before deploy. */
export function needsAcknowledgement(
  requirements: TemplateRequirements | undefined,
): boolean {
  return requirements?.level === 'elevated'
}
