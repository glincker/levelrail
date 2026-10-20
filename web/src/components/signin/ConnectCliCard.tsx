import { useEffect, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  CopyIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBrand } from '../../hooks/useBrand'
import { createCLIPairing, type CLIPairing } from '../../queries/signIn'
import { RateLimitError } from '../../queries/auth'

const DISMISS_KEY = 'connect-cli-dismissed'

function readDismissed(): boolean {
  try {
    return window.localStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

function formatLeft(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const m = Math.floor(total / 60)
  const s = String(total % 60).padStart(2, '0')
  return `${m}:${s}`
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) {
      return
    }
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [active])
  return now
}

// The command is the one thing on this card: a single-use pairing code,
// issued only to this signed-in session, expiring in minutes.
export function ConnectCliCard() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const brand = useBrand()
  const [dismissed, setDismissed] = useState(readDismissed)
  const [pairing, setPairing] = useState<CLIPairing | null>(null)
  const [copied, setCopied] = useState(false)
  const now = useNow(pairing !== null)
  const create = useMutation({
    mutationFn: createCLIPairing,
    onSuccess: (p) => {
      setPairing(p)
      setCopied(false)
    },
    onError: (err) => {
      toast.add({
        title:
          err instanceof RateLimitError
            ? t('connectCli.rateLimited')
            : t('connectCli.failed'),
        type: 'error',
      })
    },
  })
  if (dismissed) {
    return null
  }
  const left = pairing ? new Date(pairing.expires_at).getTime() - now : 0
  const live = pairing !== null && left > 0
  const command = pairing
    ? `${brand.BinaryName} login --api-url ${pairing.server_url} --code ${pairing.code}`
    : ''
  const copy = () => {
    navigator.clipboard.writeText(command).then(
      () => {
        setCopied(true)
        toast.add({ title: t('connectCli.copied'), type: 'success' })
      },
      () => {
        toast.add({ title: t('connectCli.copyFailed'), type: 'error' })
      },
    )
  }
  return (
    <section
      aria-labelledby="connect-cli-title"
      className="space-y-3 rounded-lg border bg-card p-4"
    >
      <div className="flex items-center gap-2">
        <TerminalWindowIcon aria-hidden="true" className="size-5 shrink-0" />
        <h2 id="connect-cli-title" className="flex-1 font-semibold">
          {t('connectCli.title')}
        </h2>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            try {
              window.localStorage.setItem(DISMISS_KEY, '1')
            } catch {
              // Dismissal then lasts only until the next reload.
            }
            setDismissed(true)
          }}
        >
          {t('connectCli.dismiss')}
        </Button>
      </div>
      <p className="text-sm text-muted-foreground">{t('connectCli.lead')}</p>
      <div aria-live="polite" className="space-y-2">
        {live ? (
          <>
            <code
              aria-label={t('connectCli.commandLabel')}
              data-testid="connect-cli-command"
              className="block select-all overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-sm"
            >
              {command}
            </code>
            <p className="text-xs text-muted-foreground">
              {t('connectCli.expiresIn', { time: formatLeft(left) })}
            </p>
          </>
        ) : pairing ? (
          <p className="text-sm text-muted-foreground">
            {t('connectCli.expired')}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-2">
        {live ? (
          <Button type="button" size="sm" onClick={copy}>
            {copied ? (
              <CheckIcon aria-hidden="true" className="size-4" />
            ) : (
              <CopyIcon aria-hidden="true" className="size-4" />
            )}
            {t('connectCli.copy')}
          </Button>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant={live ? 'outline' : 'default'}
          disabled={create.isPending}
          onClick={() => {
            create.mutate()
          }}
        >
          {pairing ? t('connectCli.regenerate') : t('connectCli.generate')}
        </Button>
      </div>
    </section>
  )
}
