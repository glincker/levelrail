import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { GuidedEmptyState } from './GuidedEmptyState'

export const CERTIFICATES_CLI_ARGS = 'domains add web app.example.com'

/** CertificatesEmptyState guides a truly empty certificates list to its first item. */
export function CertificatesEmptyState({ total }: { total: number }) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <GuidedEmptyState
      total={total}
      icon={<GlobeGlassIcon size="1.4em" />}
      title={t('guided.certificates.title')}
      description={t('guided.certificates.body')}
      cliArgs={CERTIFICATES_CLI_ARGS}
      helpPath="/domains-and-ingress"
      action={
        <Button size="sm" render={<Link to="/domains" />} nativeButton={false}>
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('guided.certificates.action')}
        </Button>
      }
    />
  )
}
