import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LightbulbGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useAdvisorFindings } from '../../queries/resourceAdvisor'
import { splitPending } from '../../lib/resourceAdvisor'
import type { AdvisorFinding } from '../../types/resourceAdvisor'
import { ApplyFindingDialog } from './ApplyFindingDialog'

function Row({ finding }: { finding: AdvisorFinding }) {
  const { t } = useTranslation('appAdvisor')
  const [open, setOpen] = useState(false)
  return (
    <li className="flex items-center justify-between gap-3 text-sm">
      <span className="min-w-0 truncate">{finding.title}</span>
      {finding.can_apply ? (
        <>
          <Button
            size="sm"
            variant="outline"
            aria-label={t('applyLabel', { title: finding.title })}
            onClick={() => setOpen(true)}
          >
            {t('apply')}
          </Button>
          <ApplyFindingDialog
            finding={finding}
            open={open}
            onOpenChange={setOpen}
          />
        </>
      ) : null}
    </li>
  )
}

// Shows this app's advisor findings with the same preview-then-apply flow as
// `advisor apply`; renders nothing while the beta is off or there is no advice.
export function AppAdvisorCard({ name }: { name: string }) {
  const { t } = useTranslation('appAdvisor')
  const { data } = useAdvisorFindings(`app:${name}`)
  const advice = data ? splitPending(data.findings).advice : []
  if (advice.length === 0) return null
  return (
    <section
      className="space-y-2 rounded-lg border p-3"
      data-testid="app-advisor-card"
    >
      <h3 className="flex items-center gap-2 text-sm font-medium">
        <LightbulbGlassIcon className="size-5" />
        {t('title')}
      </h3>
      <ul className="space-y-2">
        {advice.map((f) => (
          <Row key={f.id} finding={f} />
        ))}
      </ul>
      <Link
        to="/apps/$name/resources"
        params={{ name }}
        className="text-sm font-medium text-foreground underline-offset-4 hover:underline"
      >
        {t('all')}
      </Link>
    </section>
  )
}
