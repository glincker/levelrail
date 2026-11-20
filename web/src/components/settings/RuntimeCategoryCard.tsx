import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

/** RuntimeCategoryCard folds one category so 60 rows do not stack into one long page. */
export function RuntimeCategoryCard({
  id,
  title,
  count,
  weaker,
  forceOpen,
  defaultOpen,
  children,
}: {
  id: string
  title: string
  count: number
  /** Settings in this category held below their safe default. */
  weaker: number
  /** Open while a search, filter or category jump is active. */
  forceOpen: boolean
  defaultOpen: boolean
  children: ReactNode
}) {
  const { t } = useTranslation('runtimeSettings')
  const [chosen, setChosen] = useState<boolean | undefined>(undefined)
  const open = forceOpen || (chosen ?? defaultOpen)
  return (
    <Card id={id} className="scroll-mt-28">
      <Collapsible
        open={open}
        onOpenChange={(next) => {
          setChosen(next)
        }}
      >
        <CollapsibleTrigger
          disabled={forceOpen}
          className="flex items-center gap-2 px-(--card-spacing) text-left"
        >
          <h2 className="text-base font-semibold text-foreground">{title}</h2>
          <Badge variant="muted">{count}</Badge>
          {weaker > 0 ? (
            <Badge variant="destructive">
              {t('category.weaker', { count: weaker })}
            </Badge>
          ) : null}
          <ChevronDownGlassIcon
            aria-hidden="true"
            className={cn(
              'ml-auto size-5 text-muted-foreground transition-transform duration-200 motion-reduce:transition-none',
              open && 'rotate-180',
            )}
          />
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <CardContent className="pt-(--card-spacing)">{children}</CardContent>
        </CollapsiblePanel>
      </Collapsible>
    </Card>
  )
}
