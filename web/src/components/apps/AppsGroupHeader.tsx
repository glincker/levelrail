import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

// Header row for one group: collapse toggle, name, count and a group bulk select.
export function AppsGroupHeader({
  label,
  count,
  selectedCount,
  collapsed,
  onToggle,
  onSelectGroup,
}: {
  label: string
  count: number
  selectedCount: number
  collapsed: boolean
  onToggle: () => void
  onSelectGroup: (checked: boolean) => void
}) {
  const { t } = useTranslation('appRows')
  const all = selectedCount === count
  return (
    <div className="flex items-center justify-between gap-2 border-b border-border pb-1">
      <button
        type="button"
        aria-expanded={!collapsed}
        onClick={onToggle}
        className="flex min-w-0 items-center gap-2 rounded-md text-sm font-medium text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
      >
        <ChevronDownGlassIcon
          className={cn(
            'size-5 transition-transform',
            collapsed && '-rotate-90',
          )}
          aria-hidden="true"
        />
        <span className="truncate">{label}</span>
        <span className="text-xs font-normal text-muted-foreground">
          {t('group.count', { count })}
        </span>
      </button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onClick={() => {
          onSelectGroup(!all)
        }}
      >
        {all ? t('group.deselect') : t('group.select')}
      </Button>
    </div>
  )
}
