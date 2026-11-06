import { cloneElement, type MouseEvent, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { useAbilities } from '@/hooks/useAbilities'
import type { Ability } from '@/types/token'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

interface GateableProps {
  onClick?: (event: MouseEvent<HTMLElement>) => void
  'aria-disabled'?: boolean
}

interface RoleGateProps {
  ability: Ability
  children: ReactElement<GateableProps>
}

// Root and write:sensitive are admin only in practice; the rest is operator work.
function neededKey(ability: Ability) {
  return ability === 'root' || ability === 'write:sensitive'
    ? 'roleGate.needsAdmin'
    : 'roleGate.needsOperator'
}

/**
 * RoleGate keeps its child in place but aria-disabled, with a tooltip naming
 * the role needed, when the user lacks the ability. The child stays focusable
 * so the tooltip is reachable by keyboard; the element type is untouched, so
 * Base UI nativeButton rules keep applying to it.
 */
export function RoleGate({ ability, children }: RoleGateProps) {
  const { t } = useTranslation('common')
  const { can } = useAbilities()
  if (can(ability)) return children
  const blocked = cloneElement(children, {
    'aria-disabled': true,
    onClick: (event: MouseEvent<HTMLElement>) => {
      event.preventDefault()
      event.stopPropagation()
    },
  })
  return (
    <Tooltip>
      <TooltipTrigger render={<span className="inline-flex" />}>
        {blocked}
      </TooltipTrigger>
      <TooltipContent>{t(neededKey(ability))}</TooltipContent>
    </Tooltip>
  )
}
