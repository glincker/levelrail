import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useMintSiteHook,
  useRevokeSiteHook,
  useSiteHook,
} from '../../queries/pagesHook'

/** Secret redeploy URL of a git-connected site. The URL is held in state only, never refetched. */
export default function SiteHookCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const q = useSiteHook(name)
  const mint = useMintSiteHook(name)
  const revoke = useRevokeSiteHook(name)
  const [shown, setShown] = useState('')
  const [copied, setCopied] = useState(false)
  const enabled = q.data?.enabled === true
  const error = mint.error ?? revoke.error

  function onMint() {
    if (enabled && !window.confirm(t('hook.confirmRotate'))) return
    mint.mutate(undefined, {
      onSuccess: (h) => {
        setShown(h.url ?? '')
        setCopied(false)
      },
    })
  }

  function onCopy() {
    void navigator.clipboard.writeText(shown).then(() => setCopied(true))
  }

  return (
    <section
      aria-labelledby="pages-hook-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2
          id="pages-hook-title"
          className="text-sm font-semibold"
          title={t('hook.tip')}
        >
          {t('hook.title')}
        </h2>
        {q.isSuccess ? (
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={mint.isPending}
              title={enabled ? t('hook.rotateTip') : t('hook.createTip')}
              onClick={onMint}
            >
              {enabled ? t('hook.rotate') : t('hook.create')}
            </Button>
            {enabled ? (
              <Button
                size="sm"
                variant="outline"
                disabled={revoke.isPending}
                title={t('hook.revokeTip')}
                onClick={() => {
                  if (window.confirm(t('hook.confirmRevoke'))) {
                    setShown('')
                    revoke.mutate()
                  }
                }}
              >
                {t('hook.revoke')}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      {q.isPending ? <Skeleton className="h-6 w-full" /> : null}
      {q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('hook.failed')}</AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
      {shown ? (
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1 text-xs">
            {shown}
          </code>
          <Button
            size="sm"
            variant="outline"
            title={t('hook.copyTip')}
            onClick={onCopy}
          >
            {copied ? t('hook.copied') : t('hook.copy')}
          </Button>
        </div>
      ) : null}
      {enabled && !shown && q.data?.created_at ? (
        <p className="text-xs text-muted-foreground">
          {t('hook.since', {
            date: new Date(q.data.created_at).toLocaleDateString(),
          })}
        </p>
      ) : null}
      {shown ? (
        <p className="text-xs text-muted-foreground">{t('hook.shownOnce')}</p>
      ) : null}
    </section>
  )
}
