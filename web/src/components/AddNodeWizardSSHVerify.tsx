import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { FieldLabel } from '@/components/ui/field'
import type { SSHHostKey } from '../types/nodeSSHProvision'

const HOST_KEY_COMMAND = 'ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub'

// SSHHostKeyVerify shows the keys the credential-free preflight read and
// gates continuing on an explicit operator confirmation.
export function SSHHostKeyVerify({
  host,
  port,
  keys,
  selected,
  onSelect,
  confirmed,
  onConfirmedChange,
}: {
  host: string
  port: number
  keys: SSHHostKey[]
  selected: string
  onSelect: (fingerprint: string) => void
  confirmed: boolean
  onConfirmedChange: (confirmed: boolean) => void
}) {
  const { t } = useTranslation('nodes')
  const [copied, setCopied] = useState<string | null>(null)

  if (keys.length === 0) {
    return <p className="text-sm text-destructive">{t('sshVerify.noKeys')}</p>
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {t('sshVerify.description', { host, port })}
      </p>
      <fieldset className="space-y-2">
        <legend className="mb-2 text-sm font-medium">
          {t('sshVerify.keyLegend')}
        </legend>
        {keys.map((k) => (
          <label
            key={k.fingerprint}
            className={`flex items-center gap-3 rounded-lg border p-3 ${
              selected === k.fingerprint
                ? 'border-primary bg-primary/5'
                : 'border-border'
            }`}
          >
            <input
              type="radio"
              name="ssh-host-key"
              value={k.fingerprint}
              checked={selected === k.fingerprint}
              onChange={() => {
                onSelect(k.fingerprint)
                onConfirmedChange(false)
              }}
              className="accent-primary"
            />
            <ShieldCheckGlassIcon className="size-4.5 shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1">
              <span className="block text-xs text-muted-foreground">
                {k.type}
              </span>
              <code className="block font-mono text-xs break-all">
                {k.fingerprint}
              </code>
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={
                copied === k.fingerprint
                  ? t('sshVerify.copied')
                  : t('sshVerify.copy')
              }
              onClick={(e) => {
                e.preventDefault()
                void navigator.clipboard.writeText(k.fingerprint).then(() => {
                  setCopied(k.fingerprint)
                })
              }}
            >
              {copied === k.fingerprint ? (
                <CheckGlassIcon size="1.2em" />
              ) : (
                <CopyGlassIcon size="1.2em" />
              )}
            </Button>
          </label>
        ))}
      </fieldset>
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">
          {t('sshVerify.compareHint')}
        </p>
        <code className="block rounded-md bg-muted px-2 py-1 font-mono text-xs break-all">
          {HOST_KEY_COMMAND}
        </code>
      </div>
      <div className="flex items-start gap-2">
        <Checkbox
          id="ssh-host-key-confirm"
          checked={confirmed}
          onCheckedChange={(v) => onConfirmedChange(v === true)}
          className="mt-0.5"
        />
        <FieldLabel htmlFor="ssh-host-key-confirm" className="font-normal">
          {t('sshVerify.confirm')}
        </FieldLabel>
      </div>
    </div>
  )
}
