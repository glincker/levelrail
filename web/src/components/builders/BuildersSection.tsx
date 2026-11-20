import { useTranslation } from 'react-i18next'
import { HammerGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent } from '@/components/ui/card'
import { InfoTip } from '../kit/InfoTip'
import { useBuilders } from '../../queries/builders'
import { AddBuilderButton } from './AddBuilderButton'
import { BuilderCard } from './BuilderCard'

// Builders come from the builder read model; the control plane is not listed.
export function BuildersSection() {
  const { t } = useTranslation('nodes')
  const { data, isSuccess } = useBuilders()
  const builders = data?.builders ?? []

  return (
    <section aria-labelledby="builders-title" className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <h2
          id="builders-title"
          className="flex items-center gap-2 text-sm font-semibold"
        >
          <HammerGlassIcon aria-hidden="true" className="size-4" />
          {t('builders.title')}
          <InfoTip label={t('builders.tipLabel')}>{t('builders.tip')}</InfoTip>
        </h2>
        {builders.length > 0 ? (
          <span className="ml-auto">
            <AddBuilderButton />
          </span>
        ) : null}
      </div>
      {isSuccess && builders.length === 0 ? (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-3">
            <p className="flex-1 basis-56 text-sm text-muted-foreground">
              {t('builders.empty')}
            </p>
            <span className="flex items-center gap-1.5">
              <AddBuilderButton variant="default" />
              <InfoTip label={t('builders.emptyTipLabel')}>
                {t('builders.emptyTip')}
              </InfoTip>
            </span>
          </CardContent>
        </Card>
      ) : (
        <ul className="grid gap-2 lg:grid-cols-2 2xl:grid-cols-3">
          {builders.map((b) => (
            <BuilderCard key={b.id} builder={b} />
          ))}
        </ul>
      )}
    </section>
  )
}
