import { useContext, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { useDemoInfo, useIsDemo } from '@/hooks/useDemo'
import { BrandContext } from '@/lib/brandContext'

type Reason = 'terminal' | 'database' | 'generic'

/** DemoUnavailable replaces a host-reaching surface with an explanation while in demo mode. */
export function DemoUnavailable({ reason = 'generic' }: { reason?: Reason }) {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const brand = useContext(BrandContext)
  const brandName = brand?.ShortName || brand?.Name || ''
  return (
    <EmptyState
      icon={<LockGlassIcon className="size-4.5" />}
      title={t('unavailable.title')}
      description={t(`unavailable.${reason}`)}
      hint={t('unavailable.installHint', { brand: brandName })}
      action={
        info.docsUrl ? (
          <a
            href={info.docsUrl}
            target="_blank"
            rel="noreferrer"
            className="text-sm underline"
          >
            {t('unavailable.install')}
          </a>
        ) : undefined
      }
    />
  )
}

/** DemoGate renders children normally and DemoUnavailable on a demo instance. */
export function DemoGate({
  reason,
  children,
}: {
  reason?: Reason
  children: ReactNode
}) {
  const isDemo = useIsDemo()
  return isDemo ? <DemoUnavailable reason={reason} /> : <>{children}</>
}
