import * as React from 'react'

import { cn } from '@/lib/utils'

// Six form editors (HealthCheckEditor, DomainEditor, ResourceLimitsEditor,
// EnvEditor, SecretsEditor, DeployTriggerForm) each independently
// hand-rolled the same post-mutation success text with the same
// text-xs text-green-700 dark:text-green-400 classes, the same category
// of duplication badge.tsx's own history documents for status colors.
// className is passed through and merged last so each call site can still
// express its own margin need (some sit inside a space-y-* form and need
// none, others sit outside one and need mt-2/mt-3) without this component
// hard-coding a single spacing choice.
function SuccessMessage({ className, ...props }: React.ComponentProps<'p'>) {
  return (
    <p
      data-slot="success-message"
      className={cn('text-xs text-green-700 dark:text-green-400', className)}
      {...props}
    />
  )
}

export { SuccessMessage }
