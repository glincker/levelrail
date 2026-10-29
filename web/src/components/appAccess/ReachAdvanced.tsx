import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

/** Closed-by-default disclosure for addresses most people never need. */
export function ReachAdvanced({
  defaultOpen,
  children,
}: {
  defaultOpen: boolean
  children: React.ReactNode
}) {
  const { t } = useTranslation('appAccess')
  return (
    <Collapsible defaultOpen={defaultOpen}>
      <CollapsibleTrigger className="group/adv flex items-center gap-2 rounded-md py-2 text-sm font-medium focus-visible:outline-2 focus-visible:outline-ring">
        <ChevronDownGlassIcon
          className="size-5 -rotate-90 text-muted-foreground motion-safe:transition-transform group-data-[panel-open]/adv:rotate-0"
          aria-hidden="true"
        />
        {t('advanced.title')}
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <p className="pb-1 text-xs text-muted-foreground">
          {t('advanced.hint')}
        </p>
        <ul className="divide-y divide-border">{children}</ul>
      </CollapsiblePanel>
    </Collapsible>
  )
}
