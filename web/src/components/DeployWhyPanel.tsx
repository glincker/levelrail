import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { useDeployWhy } from '../queries/deployWhy'

// Panel budget: verdict, exit, health, then the top clusters up to this many lines in all.
const MAX_LINES = 5

/** DeployWhyPanel answers "why did this deploy fail" from the stored verdict, loaded on open. */
export function DeployWhyPanel({
  appName,
  deployId,
}: {
  appName: string
  deployId: string
}) {
  const { t } = useTranslation('gitBuild')
  const [open, setOpen] = useState(false)
  const bodyId = useId()
  const { data, isPending, isError } = useDeployWhy(deployId, open)

  const lines: string[] = []
  if (data) {
    lines.push(t(`why.verdict.${data.verdict}`, { defaultValue: data.verdict }))
    if (data.exit_code !== undefined) {
      lines.push(t('why.exit', { code: data.exit_code }))
    }
    if (data.health) {
      lines.push(
        t(data.health.success ? 'why.healthOk' : 'why.healthFailed', {
          target: data.health.target,
        }),
      )
    }
    for (const tpl of data.new_templates) {
      lines.push(t('why.template', { count: tpl.count, sample: tpl.sample }))
    }
  }
  const shown = lines.slice(0, MAX_LINES)

  return (
    <div className="mt-1.5">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={bodyId}
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 rounded text-xs font-medium text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
      >
        <ChevronRightGlassIcon
          aria-hidden="true"
          className={`size-4.5 transition-transform motion-reduce:transition-none ${open ? 'rotate-90' : ''}`}
        />
        {t('why.title')}
      </button>
      <div id={bodyId} hidden={!open} className="mt-1 text-xs">
        {open && isPending ? (
          <p className="text-muted-foreground">{t('why.loading')}</p>
        ) : null}
        {open && isError ? (
          <p className="text-muted-foreground">{t('why.failed')}</p>
        ) : null}
        {data ? (
          <ul className="space-y-0.5 break-words">
            {shown.map((l, i) => (
              <li key={i} className="line-clamp-2">
                {l}
              </li>
            ))}
          </ul>
        ) : null}
        {data ? (
          <Link
            to="/apps/$name/deploys/$deployId/logs"
            params={{ name: appName, deployId }}
            className="mt-1 inline-block font-medium underline underline-offset-2"
          >
            {t('why.rawLogs')}
          </Link>
        ) : null}
      </div>
    </div>
  )
}
