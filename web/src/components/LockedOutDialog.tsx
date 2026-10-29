import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  KeyGlassIcon,
  ShieldCheckGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { LifebuoyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useBrand } from '../hooks/useBrand'

interface Route {
  id: 'password' | 'token' | 'approve'
  icon: typeof KeyGlassIcon
  command: string
}

function CopyLine({ value }: { value: string }) {
  const { t } = useTranslation('common')
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-center gap-2 rounded-md bg-muted px-3 py-2">
      <code className="min-w-0 flex-1 overflow-x-auto font-mono text-xs text-foreground">
        {value}
      </code>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('lockedOut.copy')}
        onClick={() => {
          void navigator.clipboard
            .writeText(value)
            .then(() => {
              setCopied(true)
              window.setTimeout(() => {
                setCopied(false)
              }, 1500)
            })
            .catch(() => undefined)
        }}
      >
        {copied ? (
          <CheckGlassIcon size="1.2em" aria-hidden="true" />
        ) : (
          <CopyGlassIcon size="1.2em" aria-hidden="true" />
        )}
      </Button>
    </div>
  )
}

/** LockedOutDialog keeps the host recovery commands behind a link instead of printing them on the sign-in page. */
export function LockedOutDialog() {
  const { t } = useTranslation('common')
  const brand = useBrand()
  const bin = brand.BinaryName || 'server-binary'
  const routes: Route[] = [
    {
      id: 'password',
      icon: KeyGlassIcon,
      command: `sudo ${bin} recover-admin --username <your username>`,
    },
    {
      id: 'token',
      icon: ShieldCheckGlassIcon,
      command: `sudo ${bin} emergency-token --ttl 1h`,
    },
    {
      id: 'approve',
      icon: TerminalGlassIcon,
      command: `sudo ${bin} approve-signin <request id> --match <number>`,
    },
  ]

  return (
    <Dialog>
      <DialogTrigger
        render={
          <button
            type="button"
            className="inline-flex items-center gap-1.5 text-xs text-muted-foreground underline underline-offset-4 hover:text-foreground"
          />
        }
      >
        <LifebuoyIcon className="size-3.5" aria-hidden="true" />
        {t('lockedOut.link')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('lockedOut.title')}</DialogTitle>
          <DialogDescription>{t('lockedOut.description')}</DialogDescription>
        </DialogHeader>
        <ol className="space-y-3">
          {routes.map((r, i) => (
            <li
              key={r.id}
              className="space-y-2 rounded-lg border border-border p-3"
            >
              <div className="flex items-start gap-3">
                <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                  <r.icon className="size-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <p className="text-sm font-medium text-foreground">
                    {i + 1}. {t(`lockedOut.routes.${r.id}.title`)}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t(`lockedOut.routes.${r.id}.body`)}
                  </p>
                </div>
              </div>
              <CopyLine value={r.command} />
            </li>
          ))}
        </ol>
        <p className="text-xs text-muted-foreground">
          {t('lockedOut.footer')}
          {brand.DocsURL ? (
            <>
              {' '}
              <a
                href={`${brand.DocsURL}/emergency-access`}
                target="_blank"
                rel="noreferrer"
                className="underline"
              >
                {t('loginHelp.emergencyDocs')}
              </a>
            </>
          ) : null}
        </p>
      </DialogContent>
    </Dialog>
  )
}
