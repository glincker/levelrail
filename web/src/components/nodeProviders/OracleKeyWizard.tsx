import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronRightGlassIcon,
  CircleCheckGlassIcon,
  CopyGlassIcon,
  PlugGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { generateOciKeyPair } from '@/lib/ociKey'
import { useNodeProviderRegions } from '../../queries/nodeProvision'

// OCI policy syntax and CLI commands are literal, not translatable copy.
const POLICY = [
  'Allow group levelrail-provisioners to manage instance-family in compartment <name>',
  'Allow group levelrail-provisioners to use virtual-network-family in compartment <name>',
].join('\n')

const CLI = [
  'oci iam group create --name levelrail-provisioners --description "Levelrail provisioning"',
  'oci iam user create --name levelrail-provisioner --description "Levelrail provisioning"',
  'oci iam group add-user --group-id <group ocid> --user-id <user ocid>',
  'oci iam policy create --compartment-id <compartment ocid> --name levelrail-provisioners \\',
  '  --description "Levelrail provisioning" \\',
  `  --statements '["Allow group levelrail-provisioners to manage instance-family in compartment <name>","Allow group levelrail-provisioners to use virtual-network-family in compartment <name>"]'`,
  'oci iam user api-key upload --user-id <user ocid> --key-file levelrail-public.pem',
].join('\n')

interface Generated {
  publicKeyPem: string
  fingerprint: string
}

// The private key is held in state only until the click that hands it to
// the credential form via onUseKey, then cleared. It is never persisted.
export function OracleKeyWizard({
  connected,
  onUseKey,
}: {
  connected: boolean
  onUseKey: (privateKeyPem: string, fingerprint: string) => void
}) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const [generated, setGenerated] = useState<Generated | null>(null)
  const [privatePem, setPrivatePem] = useState('')
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const [used, setUsed] = useState(false)
  const [copied, setCopied] = useState('')
  const [checked, setChecked] = useState(false)
  const regions = useNodeProviderRegions('oracle', checked && connected)

  async function generate() {
    setBusy(true)
    setFailed(false)
    setUsed(false)
    try {
      const pair = await generateOciKeyPair()
      setGenerated({
        publicKeyPem: pair.publicKeyPem,
        fingerprint: pair.fingerprint,
      })
      setPrivatePem(pair.privateKeyPem)
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }

  function useKey() {
    if (!generated) return
    onUseKey(privatePem, generated.fingerprint)
    setPrivatePem('')
    setUsed(true)
  }

  function copy(id: string, text: string) {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(id)
      window.setTimeout(() => setCopied(''), 1500)
    })
  }

  function check() {
    if (checked) void regions.refetch()
    else setChecked(true)
  }

  const copyButton = (id: string, text: string) => (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={() => copy(id, text)}
    >
      <CopyGlassIcon size="1.4em" aria-hidden="true" />
      {copied === id ? t('oracle.wizard.copied') : t('oracle.wizard.copy')}
    </Button>
  )

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-lg border border-border bg-muted/30 p-3"
    >
      <CollapsibleTrigger className="flex items-center gap-2 text-sm font-medium">
        <ChevronRightGlassIcon
          size="1.4em"
          aria-hidden="true"
          className={
            open
              ? 'rotate-90 transition-transform motion-reduce:transition-none'
              : 'transition-transform motion-reduce:transition-none'
          }
        />
        {t('oracle.wizard.title')}
      </CollapsibleTrigger>
      <CollapsiblePanel className="space-y-4 pt-3">
        <p className="text-xs text-muted-foreground">
          {t('oracle.wizard.intro')}
        </p>

        <section aria-label={t('oracle.wizard.keyTitle')} className="space-y-2">
          <h4 className="text-sm font-medium">{t('oracle.wizard.keyTitle')}</h4>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => void generate()}
            >
              {busy
                ? t('oracle.wizard.generating')
                : generated
                  ? t('oracle.wizard.regenerate')
                  : t('oracle.wizard.generate')}
            </Button>
            {generated ? (
              <Button type="button" size="sm" disabled={used} onClick={useKey}>
                {used ? t('oracle.wizard.used') : t('oracle.wizard.use')}
              </Button>
            ) : null}
          </div>
          {failed ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>{t('oracle.wizard.failed')}</AlertDescription>
            </Alert>
          ) : null}
          {generated ? (
            <div className="space-y-2">
              <p className="text-xs text-muted-foreground">
                {t('oracle.wizard.consoleSteps')}
              </p>
              <div className="flex items-start gap-2">
                <pre
                  aria-label={t('oracle.wizard.publicKey')}
                  className="min-w-0 flex-1 overflow-x-auto rounded-md bg-background p-2 font-mono text-xs"
                >
                  {generated.publicKeyPem}
                </pre>
                {copyButton('pub', generated.publicKeyPem)}
              </div>
              <p className="text-xs">
                <span className="text-muted-foreground">
                  {t('oracle.wizard.fingerprint')}{' '}
                </span>
                <code className="font-mono">{generated.fingerprint}</code>
              </p>
              <p className="text-xs text-muted-foreground">
                {t('oracle.wizard.privateNote')}
              </p>
            </div>
          ) : null}
        </section>

        <section
          aria-label={t('oracle.wizard.policyTitle')}
          className="space-y-2"
        >
          <h4 className="text-sm font-medium">
            {t('oracle.wizard.policyTitle')}
          </h4>
          <p className="text-xs text-muted-foreground">
            {t('oracle.wizard.policyNote')}
          </p>
          <div className="flex items-start gap-2">
            <pre className="min-w-0 flex-1 overflow-x-auto rounded-md bg-background p-2 font-mono text-xs">
              {POLICY}
            </pre>
            {copyButton('policy', POLICY)}
          </div>
          <p className="text-xs text-muted-foreground">
            {t('oracle.wizard.cliNote')}
          </p>
          <div className="flex items-start gap-2">
            <pre className="min-w-0 flex-1 overflow-x-auto rounded-md bg-background p-2 font-mono text-xs">
              {CLI}
            </pre>
            {copyButton('cli', CLI)}
          </div>
        </section>

        <section
          aria-label={t('oracle.wizard.checkTitle')}
          className="space-y-2"
        >
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!connected || regions.isFetching}
              onClick={check}
            >
              <PlugGlassIcon size="1.4em" aria-hidden="true" />
              {regions.isFetching
                ? t('oracle.wizard.checking')
                : t('oracle.wizard.check')}
            </Button>
            {!connected ? (
              <span className="text-xs text-muted-foreground">
                {t('oracle.wizard.checkNeedsSave')}
              </span>
            ) : null}
          </div>
          {regions.isSuccess ? (
            <Alert>
              <CircleCheckGlassIcon
                className="text-tone-success"
                size="1.4em"
              />
              <AlertDescription>
                {t('oracle.wizard.checkOk', { total: regions.data.length })}
                {regions.data.length > 0
                  ? ` ${regions.data.map((r) => r.id).join(', ')}`
                  : ''}
              </AlertDescription>
            </Alert>
          ) : null}
          {regions.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>{regions.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </section>
      </CollapsiblePanel>
    </Collapsible>
  )
}
