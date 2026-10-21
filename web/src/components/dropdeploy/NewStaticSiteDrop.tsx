import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GlobeSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { SitePublisher } from './SitePublisher'

/** New-app option: drop a folder or zip and get a live static site, with no git and no build. */
export function NewStaticSiteDrop() {
  const { t } = useTranslation('newApp')
  const navigate = useNavigate()
  return (
    <section aria-labelledby="new-app-drop" className="space-y-3">
      <div className="flex items-start gap-2.5">
        <GlobeSimpleIcon
          className="mt-0.5 size-5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div>
          <h2
            id="new-app-drop"
            className="text-base font-medium text-foreground"
          >
            {t('drop.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('drop.description')}
          </p>
        </div>
      </div>
      <SitePublisher
        onPublished={(appName, result) => {
          toast.add({
            title: t('toast.created', { name: appName }),
            description: t('toast.watching'),
            type: 'success',
          })
          if (result.deployId) {
            void navigate({
              to: '/apps/$name/deploys/$deployId/logs',
              params: { name: appName, deployId: result.deployId },
            })
          }
        }}
      />
    </section>
  )
}
