import { useTranslation } from 'react-i18next'
import { LinkGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'

/** Banner above anything filled from a /deploy link. Values are rendered as text only. */
export function RepoLinkNotice({ ignored }: { ignored: string[] }) {
  const { t } = useTranslation('newApp')
  return (
    <div className="space-y-3">
      <Banner
        variant="info"
        icon={<LinkGlassIcon size="1.4em" />}
        title={t('deployPage.banner')}
      >
        {t('deployPage.bannerBody')}
      </Banner>
      {ignored.length > 0 ? (
        <Banner variant="warning" title={t('deployPage.ignoredTitle')}>
          {t('deployPage.ignoredBody', { params: ignored.join(', ') })}
        </Banner>
      ) : null}
    </div>
  )
}
