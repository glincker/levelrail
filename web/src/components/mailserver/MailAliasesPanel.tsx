import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import {
  addMailAlias,
  removeMailAlias,
  useInvalidateMail,
  useMailAliases,
  useMailboxes,
  type MailDomain,
} from '../../queries/mailMailboxes'
import { FIELD_CLASS, PanelCard, Reason } from './mailPanelUi'

export function MailAliasesPanel({ domains }: { domains: MailDomain[] }) {
  const { t } = useTranslation('mailServer')
  const canWrite = useCanWriteSensitive()
  const invalidate = useInvalidateMail()
  const boxes = useMailboxes().data ?? []
  const aliases = useMailAliases(domains.map((d) => d.domain)).data ?? []
  const [local, setLocal] = useState('')
  const [domain, setDomain] = useState('')
  const [target, setTarget] = useState('')
  const [error, setError] = useState<string | null>(null)
  const chosenDomain = domain || domains[0]?.domain || ''
  const chosenTarget = target || boxes[0]?.address || ''
  const why = canWrite ? undefined : t('mailboxes.needsWrite')

  const run = async (fn: () => Promise<unknown>) => {
    setError(null)
    try {
      await fn()
      void invalidate()
    } catch (e) {
      setError(e instanceof Error ? e.message : t('mailboxes.failed'))
    }
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    void run(async () => {
      await addMailAlias(`${local.trim()}@${chosenDomain}`, [chosenTarget])
      setLocal('')
    })
  }
  return (
    <PanelCard label={t('aliases.title')} title={t('aliases.title')}>
      {boxes.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('aliases.needsMailbox')}
        </p>
      ) : (
        <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
          <div className="flex flex-col gap-1">
            <Label htmlFor="al-local" className="text-xs">
              {t('aliases.form.alias')}
            </Label>
            <Input
              id="al-local"
              className="h-8 w-32"
              value={local}
              onChange={(e) => setLocal(e.target.value)}
              placeholder={t('aliases.form.aliasHint')}
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="al-domain" className="text-xs">
              {t('mailboxes.form.domain')}
            </Label>
            <select
              id="al-domain"
              className={FIELD_CLASS}
              value={chosenDomain}
              onChange={(e) => setDomain(e.target.value)}
            >
              {domains.map((d) => (
                <option key={d.domain} value={d.domain}>
                  @{d.domain}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="al-target" className="text-xs">
              {t('aliases.form.target')}
            </Label>
            <select
              id="al-target"
              className={FIELD_CLASS}
              value={chosenTarget}
              onChange={(e) => setTarget(e.target.value)}
            >
              {boxes.map((b) => (
                <option key={b.address} value={b.address}>
                  {b.address}
                </option>
              ))}
            </select>
          </div>
          <Reason why={why}>
            <Button
              type="submit"
              size="sm"
              className="gap-1"
              disabled={!canWrite || !local.trim()}
            >
              <PlusGlassIcon size="1.1em" />
              {t('aliases.form.add')}
            </Button>
          </Reason>
        </form>
      )}
      {error && (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      )}
      {aliases.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('aliases.empty')}</p>
      ) : (
        <ul
          aria-label={t('aliases.listLabel')}
          className="divide-y divide-border text-xs"
        >
          {aliases.map((a) => (
            <li
              key={a.address}
              className="flex items-center justify-between gap-2 py-1.5"
            >
              <span>
                <span className="font-mono">{a.address}</span>
                <span className="text-muted-foreground">
                  {' '}
                  {t('aliases.to')}{' '}
                </span>
                <span className="font-mono">{a.targets.join(', ')}</span>
              </span>
              <Reason why={why}>
                <Button
                  size="sm"
                  variant="outline"
                  className="gap-1"
                  disabled={!canWrite}
                  aria-label={t('aliases.removeFor', { address: a.address })}
                  onClick={() => void run(() => removeMailAlias(a.address))}
                >
                  <TrashGlassIcon size="1.1em" />
                  {t('aliases.remove')}
                </Button>
              </Reason>
            </li>
          ))}
        </ul>
      )}
    </PanelCard>
  )
}
