import * as React from 'react'
import { Label as GlinLabel } from '@glinui/ui'

import { cn } from '@/lib/utils'

// text-inherit and leading-none keep the old look: glinui forces its own color and line height.
function Label({ className, ...props }: React.ComponentProps<'label'>) {
  return (
    <GlinLabel
      data-slot="label"
      className={cn(
        'flex items-center gap-2 text-sm leading-none font-medium text-inherit select-none group-data-[disabled=true]:pointer-events-none group-data-[disabled=true]:opacity-50 peer-disabled:cursor-not-allowed peer-disabled:opacity-50',
        className,
      )}
      {...props}
    />
  )
}

export { Label }
