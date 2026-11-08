import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { AlgorithmCards } from './AlgorithmCards'
import { HealthFields } from './HealthFields'
import { LbTextField, type LbSectionProps } from './LbField'
import { PresetPicker } from './PresetPicker'
import { ResilienceFields } from './ResilienceFields'
import { WeightSliders } from './WeightSliders'
import type { LbPreset } from './presets'

interface Props extends LbSectionProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onPreset: (preset: LbPreset) => void
}

export function LbConfigure({
  form,
  errors,
  onChange,
  open,
  onOpenChange,
  onPreset,
}: Props) {
  const { t } = useTranslation('statePlaceholders')
  const [tab, setTab] = useState('traffic')
  return (
    <section
      aria-label={t('lb.configure.section')}
      className="rounded-2xl border"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 p-3">
        <button
          type="button"
          aria-expanded={open}
          aria-controls="lb-configure-body"
          onClick={() => onOpenChange(!open)}
          className="flex min-h-(--glinr-control-h) items-center gap-2 rounded-lg px-1 text-sm font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
        >
          <ChevronDownGlassIcon
            className={cn('size-5 transition-transform', !open && '-rotate-90')}
            aria-hidden
          />
          {t('lb.configure.toggle')}
        </button>
        <PresetPicker onPick={onPreset} />
      </div>
      {open ? (
        <div id="lb-configure-body" className="border-t p-4">
          <Tabs value={tab} onValueChange={(v) => setTab(String(v))}>
            <TabsList>
              <TabsTrigger value="traffic">
                {t('lb.configure.traffic')}
              </TabsTrigger>
              <TabsTrigger value="health">
                {t('lb.configure.health')}
              </TabsTrigger>
              <TabsTrigger value="resilience">
                {t('lb.configure.resilience')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="traffic" className="space-y-4 pt-3">
              <AlgorithmCards
                value={form.algorithm}
                onChange={(algorithm) => onChange({ algorithm })}
              />
              {form.algorithm === 'cookie' ? (
                <div className="max-w-56">
                  <LbTextField
                    id="lb-cookie-name"
                    label={t('lb.configure.cookieName')}
                    info={t('lb.configure.cookieNameInfo')}
                    value={form.cookieName}
                    placeholder="lb"
                    onChange={(cookieName) => onChange({ cookieName })}
                  />
                </div>
              ) : null}
              {form.algorithm === 'weighted' ? (
                <WeightSliders
                  form={form}
                  errors={errors}
                  onChange={onChange}
                />
              ) : null}
            </TabsContent>
            <TabsContent value="health" className="pt-3">
              <HealthFields form={form} errors={errors} onChange={onChange} />
            </TabsContent>
            <TabsContent value="resilience" className="pt-3">
              <ResilienceFields
                form={form}
                errors={errors}
                onChange={onChange}
              />
            </TabsContent>
          </Tabs>
        </div>
      ) : null}
    </section>
  )
}
