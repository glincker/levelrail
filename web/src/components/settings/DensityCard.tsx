import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { setDensity, useDensity, type Density } from '@/lib/density'

const OPTIONS: Density[] = ['compact', 'comfortable']

/** DensityCard switches the app-wide spacing density, stored in this browser only. */
export function DensityCard() {
  const { t } = useTranslation('settings')
  const density = useDensity()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('generalPage.density.title')}</CardTitle>
        <CardDescription>
          {t('generalPage.density.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div
          role="group"
          aria-label={t('generalPage.density.title')}
          className="flex flex-wrap gap-2"
        >
          {OPTIONS.map((option) => (
            <Button
              key={option}
              type="button"
              size="sm"
              variant={density === option ? 'default' : 'outline'}
              aria-pressed={density === option}
              onClick={() => setDensity(option)}
            >
              {t(`generalPage.density.${option}`)}
            </Button>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
