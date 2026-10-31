import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  KeyGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  addMailAlias,
  removeMailAlias,
  useInvalidateMail,
  useMailAliases,
  type Mailbox,
} from '../../queries/mailMailboxes'
import { MailWebmailButton } from './MailWebmailButton'
import { Reason } from './mailPanelUi'

interface Props {
  box: Mailbox | null
  app: string | undefined
  teamName: string
  userName: string
  onClose: () => void
  onToggle: (m: Mailbox) => void
  onReset: (m: Mailbox) => void
  onDelete: (m: Mailbox) => void
}

/** Right-hand detail for one mailbox: actions and its aliases. No message content. */
export function MailMailboxSheet({
  box,
  app,
  teamName,
  userName,
  onClose,
  onToggle,
  onReset,
  onDelete,
}: Props) {
  const { t } = useTranslation('mailServer')
  const canWrite = useCanWriteSensitive()
  const isRoot = useIsRoot()
  const { copied, copy } = useCopyToClipboard()
  const invalidate = useInvalidateMail()
  const aliases =
    useMailAliases(box ? [box.domain] : []).data?.filter(
      (a) => box && a.targets.includes(box.address),
    ) ?? []
  const [adding, setAdding] = useState(false)
  const [local, setLocal] = useState('')
  const [error, setError] = useState<string | null>(null)
  const writeWhy = canWrite ? undefined : t('mailboxes.needsWrite')

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
    if (!box || !local.trim()) return
    void run(async () => {
      await addMailAlias(`${local.trim()}@${box.domain}`, [box.address])
      setLocal('')
      setAdding(false)
    })
  }

  return (
    <Sheet open={box !== null} onOpenChange={(o) => !o && onClose()}>
      <SheetContent>
        {box ? (
          <>
            <SheetHeader>
              <SheetTitle className="font-mono text-sm break-all">
                {box.address}
              </SheetTitle>
              <SheetDescription>
                {t('mbx.team', { team: teamName })}.{' '}
                {t('mbx.user', { user: userName })}
              </SheetDescription>
            </SheetHeader>
            <div className="flex flex-col gap-4 px-4 pb-4">
              <div className="flex flex-wrap gap-2">
                <MailWebmailButton app={app} address={box.address} />
                <Button
                  size="sm"
                  variant="outline"
                  className="gap-1.5"
                  onClick={() => copy(box.address)}
                >
                  {copied ? (
                    <CheckGlassIcon size="1.1em" />
                  ) : (
                    <CopyGlassIcon size="1.1em" />
                  )}
                  {copied ? t('mbx.copied') : t('mbx.copy')}
                </Button>
              </div>
              <div className="flex flex-wrap gap-2">
                <Reason why={writeWhy}>
                  <Button
                    size="sm"
                    variant="outline"
                    className="gap-1.5"
                    disabled={!canWrite}
                    onClick={() => onReset(box)}
                  >
                    <KeyGlassIcon size="1.1em" />
                    {t('mailboxes.reset')}
                  </Button>
                </Reason>
                <Reason why={writeWhy}>
                  <Button
                    size="sm"
                    variant="outline"
                    className="gap-1.5"
                    disabled={!canWrite}
                    onClick={() => onToggle(box)}
                  >
                    {box.suspended ? (
                      <PlayGlassIcon size="1.1em" />
                    ) : (
                      <PauseGlassIcon size="1.1em" />
                    )}
                    {box.suspended
                      ? t('mailboxes.resume')
                      : t('mailboxes.suspend')}
                  </Button>
                </Reason>
                <Reason why={isRoot ? undefined : t('mailboxes.needsRoot')}>
                  <Button
                    size="sm"
                    variant="outline"
                    className="gap-1.5 text-destructive"
                    disabled={!isRoot}
                    aria-label={t('mailboxes.deleteFor', {
                      address: box.address,
                    })}
                    onClick={() => onDelete(box)}
                  >
                    <TrashGlassIcon size="1.1em" />
                    {t('mailboxes.delete')}
                  </Button>
                </Reason>
              </div>
              {box.suspend_pending ? (
                <p role="note" className="text-xs text-tone-warning">
                  {t('mailboxes.pendingWarning')}
                </p>
              ) : null}
              <section aria-label={t('mbx.aliasesTitle')} className="space-y-2">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs font-medium text-muted-foreground">
                    {t('mbx.aliasesTitle')}
                  </h3>
                  <Reason why={writeWhy}>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      disabled={!canWrite}
                      aria-label={t('aliases.form.add')}
                      onClick={() => setAdding((v) => !v)}
                    >
                      <PlusGlassIcon size="1.2em" />
                    </Button>
                  </Reason>
                </div>
                {adding ? (
                  <form onSubmit={submit} className="flex items-center gap-2">
                    <Input
                      className="h-8 w-32"
                      aria-label={t('aliases.form.alias')}
                      value={local}
                      autoFocus
                      onChange={(e) => setLocal(e.target.value)}
                      placeholder={t('aliases.form.aliasHint')}
                    />
                    <span className="text-xs text-muted-foreground">
                      @{box.domain}
                    </span>
                  </form>
                ) : null}
                {error ? (
                  <p role="alert" className="text-xs text-tone-danger">
                    {error}
                  </p>
                ) : null}
                {aliases.length === 0 ? (
                  <p className="text-xs text-muted-foreground">
                    {t('aliases.empty')}
                  </p>
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
                        <span className="font-mono break-all">{a.address}</span>
                        <Reason why={writeWhy}>
                          <Button
                            size="icon-sm"
                            variant="ghost"
                            disabled={!canWrite}
                            aria-label={t('aliases.removeFor', {
                              address: a.address,
                            })}
                            onClick={() =>
                              void run(() => removeMailAlias(a.address))
                            }
                          >
                            <TrashGlassIcon size="1.1em" />
                          </Button>
                        </Reason>
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
