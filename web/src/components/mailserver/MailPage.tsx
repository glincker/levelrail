import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { MailGlassIcon, RocketGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { cn } from '@/lib/utils'
import type { MailCheckReport } from '@/queries/mailDns'
import { useMailStatus } from '@/queries/mailStatus'
import { MailNextStep } from './MailNextStep'
import { MailChecklist } from './MailChecklist'
import { MailDnsPanel } from './MailDnsPanel'
import { MailPortsCard } from './MailPortsCard'
import { MailSoonPanels } from './MailSoonPanels'

/** MailPage is the experimental mail hub: install link, setup checklist, domain DNS and upcoming panels. */
export function MailPage() {
  const { t } = useTranslation('mailServer')
  const [report, setReport] = useState<MailCheckReport | undefined>()
  const [domain, setDomain] = useState('')
  const [previewToken, setPreviewToken] = useState(0)
  const status = useMailStatus().data
  return (
    <PageTemplate
      eyebrow={t('eyebrow')}
      icon={<MailGlassIcon size="1.6em" />}
      title={t('title')}
      description={t('description')}
      actions={
        status?.installed ? undefined : (
          <Link
            to="/templates"
            search={{ deploy: 'mail-mox' }}
            className={cn(buttonVariants({ variant: 'outline' }), 'gap-1.5')}
            title={t('install.body')}
          >
            <RocketGlassIcon size="1.2em" />
            {t('install.action')}
          </Link>
        )
      }
    >
      <div className="space-y-3">
        <MailNextStep
          status={status}
          report={report}
          domain={domain}
          onPreview={() => setPreviewToken((n) => n + 1)}
        />
        <MailChecklist report={report} status={status} />
        <MailPortsCard />
        <MailDnsPanel
          report={report}
          onReport={setReport}
          onDomain={setDomain}
          defaults={status}
          previewToken={previewToken}
        />
        <MailSoonPanels />
      </div>
    </PageTemplate>
  )
}
