import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

/** AppearanceLinkCard points General at the Appearance page, where density and width now live. */
export function AppearanceLinkCard() {
  const { t } = useTranslation('settings')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('generalPage.appearanceLink.title')}</CardTitle>
        <CardDescription>
          {t('generalPage.appearanceLink.description')}
        </CardDescription>
        <CardAction>
          <Button
            size="sm"
            variant="outline"
            render={<Link to="/settings/appearance" />}
          >
            {t('generalPage.appearanceLink.open')}
            <ArrowRightGlassIcon size="1.4em" aria-hidden="true" />
          </Button>
        </CardAction>
      </CardHeader>
    </Card>
  )
}
