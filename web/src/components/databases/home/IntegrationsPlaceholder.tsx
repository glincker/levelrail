import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const CATEGORIES = [
  'auth',
  'storage',
  'functions',
  'realtime',
  'ai',
  'observability',
] as const

// Static category list until the integrations API lands: no connectors are
// invented, every card is a disabled "coming soon" entry.
export function IntegrationsPlaceholder() {
  const { t } = useTranslation('databases')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('home.integrations.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('home.integrations.body')}
        </p>
      </CardHeader>
      <CardContent>
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {CATEGORIES.map((c) => (
            <li
              key={c}
              className="flex items-center justify-between gap-2 rounded-md border border-border px-3 py-2 text-sm"
            >
              <span>{t(`home.integrations.categories.${c}`)}</span>
              <Badge variant="outline">{t('home.integrations.soon')}</Badge>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
