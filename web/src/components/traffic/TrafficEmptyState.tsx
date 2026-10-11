import type { ReactNode } from 'react'
import { EmptyState } from '@/components/ui/empty-state'

export interface TrafficEmptyStateProps {
  icon: ReactNode
  title: string
  description: string
  /** The single primary action; every Traffic empty state has at most one. */
  action?: ReactNode
  /** Optional low-emphasis link next to the action. */
  secondaryLink?: ReactNode
  className?: string
}

/**
 * Traffic empty-state convention: what is empty, one sentence on why it
 * matters, one primary action, no illustration.
 */
export function TrafficEmptyState({
  icon,
  title,
  description,
  action,
  secondaryLink,
  className,
}: Readonly<TrafficEmptyStateProps>) {
  return (
    <EmptyState
      icon={icon}
      title={title}
      description={description}
      action={action}
      secondaryAction={secondaryLink}
      className={className}
    />
  )
}
