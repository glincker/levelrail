'use client'

import type { ComponentProps } from 'react'
import { Separator as GlinSeparator } from '@glinui/ui'

import { cn } from '@/lib/utils'

type SeparatorProps = Omit<
  ComponentProps<typeof GlinSeparator>,
  'variant' | 'size' | 'decorative' | 'label' | 'icon'
>

// Base UI's separator is always semantic and one pixel thick, so pin the default
// 1px glinui line and keep the role.
function Separator({
  className,
  orientation = 'horizontal',
  ...props
}: SeparatorProps) {
  return (
    <GlinSeparator
      data-slot="separator"
      data-orientation={orientation}
      variant="default"
      size="sm"
      decorative={false}
      orientation={orientation}
      className={cn(
        'shrink-0 bg-border',
        orientation === 'vertical' && 'h-auto self-stretch',
        className,
      )}
      {...props}
    />
  )
}

export { Separator }
