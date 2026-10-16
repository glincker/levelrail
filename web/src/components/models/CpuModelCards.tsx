import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { StatusPill } from '@/components/kit'
import { cn } from '@/lib/utils'
import { BrandTile } from '../BrandLogoBadge'
import { TemplateLogo } from '../TemplateLogo'
import {
  formatGiB,
  formatMemoryMiB,
  sortCards,
  speedRange,
  vendorLogoId,
  verdictTone,
} from '../../lib/modelsCpu'
import type { LocalModelCard } from '../../types/modelsCpu'

function VendorMark({ card }: { card: LocalModelCard }) {
  const logo = vendorLogoId(card.vendor)
  return (
    <BrandTile name={card.vendor} label={card.label} size="sm">
      {logo ? (
        <TemplateLogo
          id={logo}
          className="size-5"
          fallback={
            <span className="font-mono text-sm font-medium uppercase">
              {card.label.slice(0, 1)}
            </span>
          }
        />
      ) : undefined}
    </BrandTile>
  )
}

// Radio group of the curated local models; arrow keys move the selection.
export function CpuModelCards({
  cards,
  value,
  onChange,
  legend,
}: {
  cards: LocalModelCard[]
  value: string
  onChange: (id: string) => void
  legend: string
}) {
  const { t } = useTranslation('models')
  return (
    <fieldset className="min-w-0">
      <legend className="mb-1.5 text-sm font-medium text-foreground">
        {legend}
      </legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {sortCards(cards).map((card) => {
          const range = speedRange(
            card.fit.tokens_per_sec_low,
            card.fit.tokens_per_sec_high,
          )
          return (
            <label
              key={card.id}
              className={cn(
                'flex cursor-pointer flex-col gap-2 rounded-lg border border-border bg-card p-3 text-sm transition-colors hover:bg-muted/50 motion-reduce:transition-none',
                'has-checked:border-primary has-checked:bg-primary/5 has-focus-visible:ring-3 has-focus-visible:ring-ring/50',
              )}
            >
              <input
                type="radio"
                name="cpu-model"
                value={card.id}
                checked={value === card.id}
                onChange={() => {
                  onChange(card.id)
                }}
                className="sr-only"
              />
              <span className="flex items-center gap-2">
                <VendorMark card={card} />
                <span className="min-w-0">
                  <span className="block truncate font-medium text-foreground">
                    {card.label}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {t('cpu.card.size', {
                      params: card.params_b,
                      quant: card.quant,
                    })}
                  </span>
                </span>
              </span>
              <span className="flex flex-wrap gap-1.5">
                <Badge variant="outline">{t(`cpu.tier.${card.tier}`)}</Badge>
                <Badge variant="outline" title={t('cpu.card.licenseTitle')}>
                  {card.license}
                </Badge>
                {card.kind === 'embedding' ? (
                  <Badge variant="outline">{t('cpu.card.embedding')}</Badge>
                ) : null}
              </span>
              <span className="text-xs text-muted-foreground">
                {t('cpu.card.resources', {
                  download: formatGiB(card.download_bytes),
                  ram: formatMemoryMiB(card.min_ram_mib),
                })}
              </span>
              <span className="flex flex-wrap items-center justify-between gap-2 text-xs">
                <span className="text-muted-foreground">
                  {range
                    ? t('cpu.card.estimate', { range })
                    : t('cpu.card.noEstimate')}
                </span>
                <StatusPill
                  size="sm"
                  tone={verdictTone(card.fit.verdict)}
                  label={t(`cpu.verdict.${card.fit.verdict}`)}
                />
              </span>
            </label>
          )
        })}
      </div>
    </fieldset>
  )
}
