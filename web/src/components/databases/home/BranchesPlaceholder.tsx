import { useTranslation } from 'react-i18next'
import { GitBranchGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const ACTIONS = ['create', 'reset', 'restore', 'expire'] as const

export function BranchesPlaceholder() {
  const { t } = useTranslation('databases')
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranchGlassIcon className="size-5" aria-hidden="true" />
          {t('home.branches.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-foreground">{t('home.branches.body')}</p>
        <p className="text-xs text-muted-foreground uppercase">
          {t('home.branches.planned')}
        </p>
        <ul className="list-inside list-disc space-y-1 text-sm text-muted-foreground">
          {ACTIONS.map((a) => (
            <li key={a}>{t(`home.branches.actions.${a}`)}</li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
