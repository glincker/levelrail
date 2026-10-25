import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// The apps and databases placed on one node, linked to their pages.
export function NodeWorkloadsCard({
  apps,
  databases,
}: {
  apps: string[]
  databases: string[]
}) {
  const { t } = useTranslation('nodes')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.placed.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        {apps.length === 0 && databases.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('detail.placed.none')}
          </p>
        ) : (
          <dl className="grid gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('detail.placed.apps')}
              </dt>
              <dd className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-sm">
                {apps.map((name) => (
                  <Link
                    key={name}
                    to="/apps/$name"
                    params={{ name }}
                    className="font-mono underline-offset-2 hover:underline"
                  >
                    {name}
                  </Link>
                ))}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('detail.placed.databases')}
              </dt>
              <dd className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-sm">
                {databases.map((name) => (
                  <Link
                    key={name}
                    to="/databases/$name"
                    params={{ name }}
                    className="font-mono underline-offset-2 hover:underline"
                  >
                    {name}
                  </Link>
                ))}
              </dd>
            </div>
          </dl>
        )}
      </CardContent>
    </Card>
  )
}
