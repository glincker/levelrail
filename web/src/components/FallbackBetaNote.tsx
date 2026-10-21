import { useTranslation } from 'react-i18next'
import { FREE_PROVIDER_HOST } from '../lib/freeDnsHost'
import { BetaBadge } from './BetaBadge'

/** FallbackBetaNote labels a free wildcard-DNS fallback address as a beta test address. */
export function FallbackBetaNote({ url }: { url?: string }) {
  const { t } = useTranslation('networkProxy')
  if (!url || !FREE_PROVIDER_HOST.test(url)) return null
  return (
    <p className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <BetaBadge hint={t('automaticDomain.betaHint')} />
      {t('automaticDomain.betaHint')}
    </p>
  )
}
