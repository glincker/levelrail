import { useContext, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { BrandContext } from '@/lib/brandContext'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

/** Wraps a disabled control so hover and focus explain it is read-only in the demo. */
export function DemoBlockedTip({ children }: { children: ReactElement }) {
  const { t } = useTranslation('demo')
  const brand = useContext(BrandContext)
  const name = brand?.ShortName || brand?.Name || ''
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className="inline-flex cursor-not-allowed" tabIndex={0} />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{t('readOnly.tooltip', { brand: name })}</TooltipContent>
    </Tooltip>
  )
}
