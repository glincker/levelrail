import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { HelpLink } from '@/components/HelpLink'

/** ThirdPartyCdnNote explains, without hype, how to put an external CDN in front. */
export function ThirdPartyCdnNote() {
  const { t } = useTranslation('cdn')
  return (
    <Banner variant="info" title={t('thirdParty.title')}>
      <ul className="list-disc space-y-1 pl-5">
        <li>{t('thirdParty.headers')}</li>
        <li>{t('thirdParty.clientIp')}</li>
        <li>{t('thirdParty.plugins')}</li>
      </ul>
      <div className="mt-2">
        <HelpLink
          path="/cdn-and-caching#putting-a-cdn-in-front"
          label={t('thirdParty.docs')}
          variant="inline"
        />
      </div>
    </Banner>
  )
}
