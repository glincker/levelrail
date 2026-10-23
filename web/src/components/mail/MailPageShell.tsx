import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { EnvelopeIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'

export type MailSection = 'templates' | 'audiences' | 'messages'

const TAB_CLASS =
  'relative -mb-px border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-[current=page]:border-foreground aria-[current=page]:font-medium aria-[current=page]:text-foreground'

/** MailPageShell is the hero and section links shared by the email templates, audiences and messages pages. */
export function MailPageShell({
  section,
  actions,
  banners,
  children,
}: {
  section: MailSection
  actions?: ReactNode
  banners?: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation('mail')
  const current = (id: MailSection) => (section === id ? 'page' : undefined)

  const tabs = (
    <nav
      aria-label={t('nav.label')}
      className="flex flex-wrap border-b border-border"
    >
      <Link to="/settings/email" className={TAB_CLASS}>
        {t('nav.delivery')}
      </Link>
      <Link
        to="/settings/email-templates"
        aria-current={current('templates')}
        className={TAB_CLASS}
      >
        {t('nav.templates')}
      </Link>
      <Link
        to="/settings/email-audiences"
        aria-current={current('audiences')}
        className={TAB_CLASS}
      >
        {t('nav.audiences')}
      </Link>
      <Link
        to="/settings/email-messages"
        aria-current={current('messages')}
        className={TAB_CLASS}
      >
        {t('nav.messages')}
      </Link>
    </nav>
  )

  return (
    <PageTemplate
      icon={<EnvelopeIcon />}
      eyebrow={t(`${section}.eyebrow`)}
      title={t(`${section}.title`)}
      description={t(`${section}.description`)}
      actions={actions}
      banners={banners}
      tabs={tabs}
      width="wide"
    >
      {children}
    </PageTemplate>
  )
}
