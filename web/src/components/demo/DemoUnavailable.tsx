import { useContext, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { useDemoInfo, useIsDemo } from '@/hooks/useDemo'
import { BrandContext } from '@/lib/brandContext'
import {
  DemoSampleChat,
  DemoSampleQuery,
  DemoSampleTerminal,
} from './DemoSamples'

type Reason = 'terminal' | 'database' | 'generic' | 'ai'

/** DemoUnavailable replaces a host-reaching surface with an explanation while in demo mode. */
export function DemoUnavailable({ reason = 'generic' }: { reason?: Reason }) {
  if (reason === 'ai') return <DemoSampleChat />
  if (reason === 'terminal') return <DemoSampleTerminal />
  if (reason === 'database') return <DemoSampleQuery />
  return <DemoUnavailableNotice />
}

function DemoUnavailableNotice() {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const brand = useContext(BrandContext)
  const brandName = brand?.ShortName || brand?.Name || ''
  return (
    <EmptyState
      icon={<LockGlassIcon className="size-5" />}
      title={t('unavailable.title')}
      description={t('unavailable.generic')}
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
