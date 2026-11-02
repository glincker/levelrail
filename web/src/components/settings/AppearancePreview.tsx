import { useTranslation } from 'react-i18next'
import { GaugeGlassIcon, RocketGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

/** AppearancePreview is a mini dashboard drawn with the live tokens, so it follows every choice as it is made. */
export function AppearancePreview() {
  const { t } = useTranslation('settings')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('appearancePage.preview.title')}</CardTitle>
        <CardDescription>
          {t('appearancePage.preview.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div
          aria-hidden="true"
          className="flex overflow-hidden rounded-lg border border-border bg-background text-foreground"
        >
          <div className="flex w-32 shrink-0 flex-col gap-1 border-r border-sidebar-border bg-sidebar p-2 text-sidebar-foreground">
            <span className="flex items-center gap-2 rounded-md bg-sidebar-active px-2 py-1 text-xs font-medium">
              <GaugeGlassIcon size="1.2em" />
              {t('appearancePage.preview.navActive')}
            </span>
            <span className="flex items-center gap-2 rounded-md px-2 py-1 text-xs text-muted-foreground">
              <RocketGlassIcon size="1.2em" />
              {t('appearancePage.preview.navOther')}
            </span>
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-2 p-3">
            <div className="rounded-lg border border-border bg-card p-3 text-card-foreground">
              <p className="text-sm font-medium">
                {t('appearancePage.preview.cardTitle')}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('appearancePage.preview.cardBody')}
              </p>
              <div className="mt-2 flex gap-2">
                <Button type="button" size="sm" tabIndex={-1}>
                  {t('appearancePage.preview.primary')}
                </Button>
                <Button type="button" size="sm" variant="outline" tabIndex={-1}>
                  {t('appearancePage.preview.secondary')}
                </Button>
              </div>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
