import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { InfoGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import type { Detection } from '../../queries/importAnalyze'

const CONFIDENCE_VARIANT = {
  high: 'success',
  medium: 'info',
  low: 'warning',
} as const

function Why({ title, lines }: { title: string; lines: string[] }) {
  const { t } = useTranslation('newApp')
  if (lines.length === 0) return null
  return (
    <Popover>
      <PopoverTrigger
        className="inline-flex items-center gap-1 text-xs text-muted-foreground underline underline-offset-2"
        aria-label={t('detection.whyFor', { field: title })}
      >
        <InfoGlassIcon className="size-5 text-tone-info" aria-hidden="true" />
        {t('detection.why')}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <PopoverTitle>{title}</PopoverTitle>
        <ul className="list-disc space-y-1 pl-4 text-xs text-muted-foreground">
          {lines.map((l) => (
            <li key={l}>{l}</li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  )
}

function Card({
  label,
  value,
  lines,
}: {
  label: string
  value: ReactNode
  lines: string[]
}) {
  return (
    <div className="space-y-1 rounded-md border border-border p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="font-mono text-sm break-all">{value}</p>
      <Why title={label} lines={lines} />
    </div>
  )
}

/**
 * One card per detected field, each with the signals that produced it.
 */
export function DetectionSummary({
  detection: d,
  hideWarnings = [],
}: {
  detection: Detection
  hideWarnings?: string[]
}) {
  const { t } = useTranslation('newApp')
  const all = d.reasons.map((r) => r.detail)
  const fromFramework = d.framework
    ? [t('detection.fromFramework', { framework: d.framework })]
    : []
  const cards: { id: string; label: string; value: string; lines: string[] }[] =
    [
      {
        id: 'framework',
        label: t('detection.framework'),
        value: [d.framework, d.language].filter((v) => v !== '').join(' / '),
        lines: all,
      },
      {
        id: 'build',
        label: t('detection.buildType'),
        value: d.build_type,
        lines: all.filter((_, i) =>
          ['dockerfile', 'file', 'lockfile'].includes(
            d.reasons[i]?.signal ?? '',
          ),
        ),
      },
      {
        id: 'port',
        label: t('detection.port'),
        value: d.port.value > 0 ? String(d.port.value) : '',
        lines: d.port.source ? [d.port.source] : [],
      },
      {
        id: 'start',
        label: t('detection.start'),
        value: d.start_command ?? '',
        lines: d.start_command ? fromFramework : [],
      },
      {
        id: 'buildCmd',
        label: t('detection.buildCommand'),
        value: d.build_command ?? '',
        lines: d.build_command ? fromFramework : [],
      },
      {
        id: 'output',
        label: t('detection.output'),
        value: d.output_dir ?? '',
        lines: d.output_dir ? fromFramework : [],
      },
      {
        id: 'runtime',
        label: t('detection.runtime'),
        value: d.runtime_version.value,
        lines: d.runtime_version.source ? [d.runtime_version.source] : [],
      },
      {
        id: 'health',
        label: t('detection.health'),
        value: d.health_path,
        lines: d.reasons
          .filter((r) => r.signal === 'healthcheck')
          .map((r) => r.detail)
          .concat(fromFramework),
      },
    ]
  return (
    <section className="space-y-3" aria-labelledby="f-detect">
      <div className="flex flex-wrap items-center gap-2">
        <h3 id="f-detect" className="text-sm font-semibold">
          {t('detection.title')}
        </h3>
        <Badge variant={CONFIDENCE_VARIANT[d.confidence]}>
          {t(`detection.confidence.${d.confidence}`)}
        </Badge>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        {cards
          .filter((c) => c.value !== '')
          .map((c) => (
            <Card key={c.id} label={c.label} value={c.value} lines={c.lines} />
          ))}
      </div>
      {d.warnings
        .filter((w) => !hideWarnings.includes(w.code))
        .map((w) => (
          <Alert key={w.code}>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
              aria-hidden="true"
            />
            <AlertDescription>
              {t(`detection.warnings.${w.code}.text`, {
                defaultValue: w.message,
              })}{' '}
              {t(`detection.warnings.${w.code}.fix`, { defaultValue: '' })}
            </AlertDescription>
          </Alert>
        ))}
    </section>
  )
}
