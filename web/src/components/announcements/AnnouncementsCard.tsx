import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ApiErrorAlert } from '@/components/ApiErrorAlert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { routeErrorMessage } from '@/lib/apiError'
import {
  useRuntimeSettings,
  useResetRuntimeSetting,
  useSetRuntimeSetting,
} from '@/queries/runtimeSettings'
import type { RuntimeSetting } from '@/types/runtimeSettings'

export const ANNOUNCEMENTS_CARD_ID = 'announcements'
export const KEY_MODE = 'updates.announcements'
export const KEY_URL = 'updates.announcements_feed_url'
export const KEY_NEWSLETTER = 'updates.announcements_newsletter'
const KEY_INTERVAL = 'updates.announcements_interval'

type Mode = 'off' | 'security' | 'all'

function find(list: RuntimeSetting[], key: string) {
  return list.find((s) => s.key === key)
}

// Opt-in for the signed feed. Off fetches nothing; the privacy text below the
// choice says exactly what a fetch reveals.
export function AnnouncementsCard() {
  const { t } = useTranslation('announcements')
  const q = useRuntimeSettings()
  const save = useSetRuntimeSetting()
  const reset = useResetRuntimeSetting()
  const [url, setUrl] = useState<string | null>(null)

  if (q.isPending) {
    return <Skeleton className="h-40 w-full" aria-hidden="true" />
  }
  if (q.isError) return <ApiErrorAlert error={q.error} />

  const list = q.data.settings
  const mode = (find(list, KEY_MODE)?.value as Mode | undefined) ?? 'off'
  const urlSetting = find(list, KEY_URL)
  const newsletter = find(list, KEY_NEWSLETTER)?.value !== false
  const interval = String(find(list, KEY_INTERVAL)?.value ?? '')
  const shownUrl = url ?? String(urlSetting?.value ?? '')

  function put(key: string, value: string | boolean, done?: () => void) {
    save.mutate(
      { key, value },
      {
        onSuccess: () => {
          toast.add({ title: t('card.saved'), type: 'success' })
          done?.()
        },
        onError: (e) =>
          toast.add({
            title: t('card.saveFailed', { message: routeErrorMessage(e) }),
            type: 'error',
          }),
      },
    )
  }

  const modes = [
    { value: 'off' as const, label: t('card.off') },
    { value: 'security' as const, label: t('card.security') },
    { value: 'all' as const, label: t('card.all') },
  ]

  return (
    <Card id={ANNOUNCEMENTS_CARD_ID} className="scroll-mt-24">
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-base font-medium">{t('card.title')}</h2>
            <p className="text-sm text-muted-foreground">
              {t('card.description')}
            </p>
          </div>
          <Button
            size="sm"
            variant="outline"
            render={<Link to="/settings/announcements" />}
          >
            {t('card.viewAll')}
          </Button>
        </div>

        <div className="grid items-center gap-3 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">
          <span className="text-sm font-medium">{t('card.mode')}</span>
          <SegmentedControl
            aria-label={t('card.modeLabel')}
            value={mode}
            options={modes}
            disabled={save.isPending}
            onValueChange={(v) => put(KEY_MODE, v)}
          />
          <span className="text-sm font-medium">{t('card.newsletter')}</span>
          <label className="flex items-center gap-2 text-sm text-muted-foreground">
            <Switch
              size="sm"
              checked={newsletter}
              disabled={save.isPending || mode !== 'all'}
              aria-label={t('card.newsletter')}
              onCheckedChange={(v) => put(KEY_NEWSLETTER, v)}
            />
            {t('card.newsletterHint')}
          </label>
        </div>

        <p className="rounded-md bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
          <span className="font-medium text-foreground">
            {t('card.reveals')}.{' '}
          </span>
          {t('card.revealsBody')}
        </p>

        <details className="group">
          <summary className="cursor-pointer text-sm font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring">
            {t('card.advanced')}
          </summary>
          <form
            className="mt-3 grid items-center gap-3 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]"
            onSubmit={(e) => {
              e.preventDefault()
              put(KEY_URL, shownUrl.trim(), () => setUrl(null))
            }}
          >
            <label htmlFor="announcements-url" className="text-sm font-medium">
              {t('card.url')}
            </label>
            <div className="space-y-1">
              <div className="flex gap-2">
                <Input
                  id="announcements-url"
                  type="url"
                  inputMode="url"
                  value={shownUrl}
                  aria-describedby="announcements-url-hint"
                  onChange={(e) => setUrl(e.target.value)}
                />
                <Button
                  type="submit"
                  size="sm"
                  disabled={save.isPending || url === null}
                >
                  {t('card.save')}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={reset.isPending || urlSetting?.source !== 'saved'}
                  onClick={() =>
                    reset.mutate(
                      { key: KEY_URL },
                      { onSuccess: () => setUrl(null) },
                    )
                  }
                >
                  {t('card.reset')}
                </Button>
              </div>
              <p
                id="announcements-url-hint"
                className="text-xs text-muted-foreground"
              >
                {t('card.urlHint')}
              </p>
            </div>
            <span className="text-sm font-medium">{t('card.interval')}</span>
            <p className="text-sm text-muted-foreground">
              <code className="font-mono">{interval}</code>{' '}
              {t('card.intervalHint')}
            </p>
          </form>
        </details>
      </CardContent>
    </Card>
  )
}
