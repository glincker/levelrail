import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CpuGlassIcon,
  DatabaseGlassIcon,
  GlobeGlassIcon,
  PlugGlassIcon,
  ServerGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { formatBytes } from '../../lib/format'
import type { TemplateSummary } from '../../lib/templateSummary'

function Item({
  icon,
  label,
  children,
}: {
  icon: ReactNode
  label: string
  children: ReactNode
}) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <span
        aria-hidden="true"
        className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground [&_svg]:size-5"
      >
        {icon}
      </span>
      <div className="min-w-0">
        <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {label}
        </dt>
        <dd className="text-sm break-words text-foreground">{children}</dd>
      </div>
    </div>
  )
}

/** The "What you will get" card: every fact comes from the template definition, nothing is guessed. */
export function TemplateWhatYouGet({
  summary,
  slug,
}: {
  summary: TemplateSummary
  slug: string
}) {
  const { t } = useTranslation('templates')
  return (
    <section
      aria-labelledby="template-what"
      className="rounded-xl border border-border bg-card p-4 sm:p-5"
    >
      <h2 id="template-what" className="text-base font-semibold">
        {t('what.title')}
      </h2>
      <p className="mb-4 text-sm text-muted-foreground">
        {t('what.description', { count: summary.serviceCount })}
      </p>
      <dl className="grid gap-4 sm:grid-cols-2">
        <Item icon={<GlobeGlassIcon size="1.4em" />} label={t('what.url')}>
          <span className="font-mono text-xs">
            {t('what.urlValue', { slug })}
          </span>
        </Item>
        <Item icon={<PlugGlassIcon size="1.4em" />} label={t('what.ports')}>
          {summary.ports.length > 0 ? (
            <span className="font-mono text-xs">
              {summary.ports.join(', ')}
            </span>
          ) : (
            t('what.portsNone')
          )}
        </Item>
        <Item icon={<CpuGlassIcon size="1.4em" />} label={t('what.resources')}>
          {summary.memoryBytes
            ? t('what.memory', { size: formatBytes(summary.memoryBytes) })
            : t('what.memoryNone')}
          {', '}
          {summary.cpus
            ? t('what.cpu', { count: summary.cpus })
            : t('what.cpuShared')}
        </Item>
        <Item icon={<ServerGlassIcon size="1.4em" />} label={t('what.volumes')}>
          {summary.volumes.length > 0 ? (
            <ul className="space-y-0.5">
              {summary.volumes.map((v) => (
                <li key={v} className="truncate font-mono text-xs">
                  {v}
                </li>
              ))}
            </ul>
          ) : (
            t('what.volumesNone')
          )}
        </Item>
        <Item
          icon={<DatabaseGlassIcon size="1.4em" />}
          label={t('what.dependencies')}
        >
          {summary.databases.length > 0
            ? t('what.databaseCreated', {
                engines: summary.databases.join(', '),
              })
            : t('what.dependenciesNone')}
        </Item>
        <Item
          icon={
            <ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />
          }
          label={t('what.backups')}
        >
          {summary.hasBackups
            ? t('what.backupsVolumes')
            : t('what.backupsNone')}
        </Item>
      </dl>
    </section>
  )
}
