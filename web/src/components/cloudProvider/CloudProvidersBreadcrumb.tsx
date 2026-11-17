import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Breadcrumb, type BreadcrumbItem } from '@/components/ui/breadcrumb'

const LINK =
  'rounded-sm text-muted-foreground hover:text-foreground hover:underline'

// Integrations > Cloud providers [> provider]: shared by the list and the landing page.
export function CloudProvidersBreadcrumb({
  current,
}: Readonly<{ current?: string }>) {
  const { t } = useTranslation('cloudProvider')
  const items: BreadcrumbItem[] = [
    {
      key: 'integrations',
      label: t('breadcrumb.integrations'),
      link: (
        <Link to="/integrations" className={LINK}>
          {t('breadcrumb.integrations')}
        </Link>
      ),
    },
    current
      ? {
          key: 'cloud',
          label: t('breadcrumb.cloudProviders'),
          link: (
            <Link to="/settings/node-providers" className={LINK}>
              {t('breadcrumb.cloudProviders')}
            </Link>
          ),
        }
      : { key: 'cloud', label: t('breadcrumb.cloudProviders') },
  ]
  if (current) items.push({ key: 'current', label: current })
  return <Breadcrumb items={items} />
}
