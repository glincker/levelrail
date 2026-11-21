import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  LOG_HOURS_DEFAULT,
  LOG_LIMIT_DEFAULT,
  METRIC_HOURS_DEFAULT,
  METRIC_NAMES_DEFAULT,
  SPARK_VIEWBOX,
  sparkPoints,
} from '../../lib/cloudAwsOps'
import {
  useAwsLogEvents,
  useAwsLogGroups,
  useAwsMetrics,
} from '../../queries/cloudAwsOps'
import { StateLine } from './AwsServiceShared'

function Logs({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const [prefix, setPrefix] = useState('')
  const [group, setGroup] = useState('')
  const [filter, setFilter] = useState('')
  const groups = useAwsLogGroups(id, prefix, LOG_LIMIT_DEFAULT)
  const events = useAwsLogEvents(
    id,
    group,
    filter,
    LOG_HOURS_DEFAULT,
    LOG_LIMIT_DEFAULT,
  )
  return (
    <section className="space-y-2" aria-label={t('services.cloudwatch.logs')}>
      <h4 className="text-xs font-medium text-muted-foreground">
        {t('services.cloudwatch.logs')}
      </h4>
      <div className="flex flex-wrap items-center gap-2">
        <Input
          className="w-44"
          value={prefix}
          aria-label={t('services.cloudwatch.prefix')}
          placeholder={t('services.cloudwatch.prefix')}
          onChange={(e) => {
            setPrefix(e.target.value)
          }}
        />
        <Input
          className="w-44"
          value={filter}
          aria-label={t('services.cloudwatch.filter')}
          placeholder={t('services.cloudwatch.filter')}
          onChange={(e) => {
            setFilter(e.target.value)
          }}
        />
      </div>
      <StateLine
        pending={groups.isPending}
        error={groups.error}
        empty={
          groups.data?.length === 0
            ? t('services.cloudwatch.noGroups')
            : undefined
        }
      />
      <ul className="space-y-1">
        {(groups.data ?? []).map((g) => (
          <li
            key={g.name}
            className="flex flex-wrap items-center gap-2 text-sm"
          >
            <span className="font-mono text-xs">{g.name}</span>
            <span className="text-xs text-muted-foreground">
              {g.retention_days > 0
                ? t('services.cloudwatch.retention', { days: g.retention_days })
                : t('services.cloudwatch.retentionNever')}
            </span>
            <Button
              size="sm"
              variant="outline"
              className="ml-auto"
              aria-pressed={group === g.name}
              onClick={() => {
                setGroup(g.name)
              }}
            >
              {t('services.cloudwatch.view')}
            </Button>
          </li>
        ))}
      </ul>
      {group === '' ? null : (
        <div className="space-y-1">
          <StateLine
            pending={events.isPending}
            error={events.error}
            empty={
              events.data?.events.length === 0
                ? t('services.cloudwatch.noEvents')
                : undefined
            }
          />
          <ul className="max-h-48 space-y-0.5 overflow-auto rounded-md border border-border p-2">
            {(events.data?.events ?? []).map((e, i) => (
              <li
                key={`${e.ts}-${String(i)}`}
                className="font-mono text-xs break-all"
              >
                <span className="text-muted-foreground">{e.ts}</span>{' '}
                {e.message}
              </li>
            ))}
          </ul>
          {events.data?.truncated ? (
            <p className="text-xs text-muted-foreground">
              {t('services.cloudwatch.truncated')}
            </p>
          ) : null}
        </div>
      )}
    </section>
  )
}

export function MetricsCard({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const [instance, setInstance] = useState('')
  const [names, setNames] = useState(METRIC_NAMES_DEFAULT)
  const [query, setQuery] = useState({ instance: '', names: '' })
  const metrics = useAwsMetrics(
    id,
    query.instance,
    query.names,
    METRIC_HOURS_DEFAULT,
  )
  return (
    <div className="space-y-3">
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setQuery({ instance: instance.trim(), names: names.trim() })
        }}
      >
        <Input
          className="w-48"
          value={instance}
          aria-label={t('services.cloudwatch.instance')}
          placeholder={t('services.cloudwatch.instance')}
          onChange={(e) => {
            setInstance(e.target.value)
          }}
        />
        <Input
          className="w-64"
          value={names}
          aria-label={t('services.cloudwatch.names')}
          onChange={(e) => {
            setNames(e.target.value)
          }}
        />
        <Button size="sm" type="submit" disabled={instance.trim() === ''}>
          {t('services.cloudwatch.load')}
        </Button>
      </form>
      {query.instance === '' ? null : (
        <StateLine
          pending={metrics.isPending}
          error={metrics.error}
          empty={
            metrics.data?.series.length === 0
              ? t('services.cloudwatch.noData')
              : undefined
          }
        />
      )}
      <ul className="space-y-1">
        {(metrics.data?.series ?? []).map((s) => {
          const last = s.points.at(-1)
          return (
            <li key={s.name} className="flex items-center gap-3 text-sm">
              <span className="w-40 text-xs">{s.name}</span>
              <svg
                role="img"
                aria-label={s.name}
                viewBox={SPARK_VIEWBOX}
                className="h-7 w-32 text-primary"
              >
                <polyline
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  points={sparkPoints(s.points.map((p) => p.v))}
                />
              </svg>
              <span className="font-mono text-xs text-muted-foreground">
                {last ? last.v.toFixed(2) : ''}
              </span>
            </li>
          )
        })}
      </ul>
      <Logs id={id} />
    </div>
  )
}
