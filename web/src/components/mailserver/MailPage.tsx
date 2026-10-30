import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { MailGlassIcon, RocketGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { cn } from '@/lib/utils'
import type { MailCheckReport } from '@/queries/mailDns'
import { MailChecklist } from './MailChecklist'
import { MailDnsPanel } from './MailDnsPanel'
import { MailSoonPanels } from './MailSoonPanels'

/** MailPage is the experimental mail hub: install link, setup checklist, domain DNS and upcoming panels. */
export function MailPage() {
  const { t } = useTranslation('mailServer')
  const [report, setReport] = useState<MailCheckReport | undefined>()
  return (
    <PageTemplate
      eyebrow={t('eyebrow')}
      icon={<MailGlassIcon size="1.6em" />}
      title={t('title')}
      description={t('description')}
      actions={
        <Link
          to="/templates"
          search={{ deploy: 'mail-mox' }}
          className={cn(buttonVariants({ variant: 'outline' }), 'gap-1.5')}
          title={t('install.body')}
        >
          <RocketGlassIcon size="1.2em" />
          {t('install.action')}
        </Link>
      }
    >
      <div className="space-y-3">
        <MailChecklist report={report} />
        <MailDnsPanel report={report} onReport={setReport} />
        <MailSoonPanels />
      </div>
    </PageTemplate>
  )
}
