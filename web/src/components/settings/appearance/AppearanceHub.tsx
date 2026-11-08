import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useTheme } from '@/components/ThemeProvider'
import { AppearancePreview } from './AppearancePreview'
import { ThemeCards } from './ThemeCards'

/** AppearanceHub is the top of the page: live preview beside the theme choice, with a quiet saved or reset notice. */
export function AppearanceHub({
  onReset,
  lookSlot,
}: {
  onReset: () => void
  /** Where the Look (glass or restrained) control mounts. */
  lookSlot?: ReactNode
}) {
  const { t } = useTranslation('settings')
  const { theme, setTheme } = useTheme()
  const [notice, setNotice] = useState('')

  return (
    <div className="grid min-w-0 gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>{t('appearancePage.hub.preview.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <AppearancePreview />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('appearancePage.hub.theme.title')}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <ThemeCards
            label={t('appearancePage.hub.theme.title')}
            value={theme}
            onChange={(next) => {
              setTheme(next)
              setNotice(t('appearancePage.hub.saved'))
            }}
          />
          {lookSlot}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p
              role="status"
              aria-live="polite"
              className="text-xs text-muted-foreground"
            >
              {notice || t('appearancePage.savedHere')}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                onReset()
                setNotice(t('appearancePage.hub.resetDone'))
              }}
            >
              <RotateCcwGlassIcon size="1.4em" aria-hidden="true" />
              {t('appearancePage.reset')}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
