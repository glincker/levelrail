import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { STANDALONE_SCOPE } from '../../lib/standaloneDomain'
import { useRunGoLive } from '../../queries/goLive'
import type { StandaloneDomain } from '../../queries/standaloneDomains'
import { GoLivePanel } from '../GoLivePanel'
import { DomainDnsSetup } from '../dns/DomainDnsSetup'
import { DomainRecords } from './ServerAddressCard'
import { StandaloneDomainForm } from './StandaloneDomainForm'

function StandaloneResult({
  saved,
  onClose,
}: {
  saved: StandaloneDomain
  onClose: () => void
}) {
  const { t } = useTranslation('domains')
  const run = useRunGoLive(STANDALONE_SCOPE, saved.domain)
  const started = useRef(false)
  // Same automation app domains get on add: create the DNS record when a
  // provider is connected. Best effort, the panel shows the outcome.
  useEffect(() => {
    if (started.current) return
    started.current = true
    run.mutate({})
  }, [run])
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('standalone.resultTitle')}</DialogTitle>
        <DialogDescription>{t('standalone.resultBody')}</DialogDescription>
      </DialogHeader>
      <p className="font-mono text-sm break-all">
        {saved.domain}
        <span className="text-muted-foreground"> {'->'} </span>
        {saved.target_url}
      </p>
      <DomainDnsSetup
        kind="app"
        domain={saved.domain}
        fallback={<DomainRecords domain={saved.domain} />}
      />
      <GoLivePanel
        app={STANDALONE_SCOPE}
        domain={saved.domain}
        initial={run.data}
      />
      <DialogFooter>
        <Button type="button" onClick={onClose}>
          {t('standalone.doneLabel')}
        </Button>
      </DialogFooter>
    </>
  )
}

// StandaloneAddBody is the "Proxy to a port or URL" side of the add dialog.
export function StandaloneAddBody({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation('domains')
  const [saved, setSaved] = useState<StandaloneDomain | null>(null)

  if (saved) return <StandaloneResult saved={saved} onClose={onClose} />

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('standalone.title')}</DialogTitle>
        <DialogDescription>{t('standalone.description')}</DialogDescription>
      </DialogHeader>
      <StandaloneDomainForm
        onSaved={(d) => {
          toast.add({
            title: t('standalone.added', {
              domain: d.domain,
              target: d.target_url,
            }),
            type: 'success',
          })
          setSaved(d)
        }}
      />
    </>
  )
}
