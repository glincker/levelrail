import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

// Domains, Certificates and DNS read as one area. Plain links, not a panel
// switcher, because each tab is its own route.
export function DomainsTabs({
  active = 'domains',
}: {
  active?: 'domains' | 'certificates' | 'dns'
}) {
  const { t } = useTranslation('domains')
  const items = [
    { to: '/domains', key: 'domains' },
    { to: '/domains/certificates', key: 'certificates' },
    { to: '/dns', key: 'dns' },
  ] as const
  return (
    <nav aria-label={t('tabs.label')} className="border-b border-border">
      <ul className="-mb-px flex gap-1 overflow-x-auto">
        {items.map((item) => (
          <li key={item.key}>
            <Link
              to={item.to}
              aria-current={item.key === active ? 'page' : undefined}
              className={cn(
                'inline-flex h-9 items-center border-b-2 px-3 text-sm font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring',
                item.key === active
                  ? 'border-primary text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              {t(`tabs.${item.key}`)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  )
}
