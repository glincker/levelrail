import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { COPY_FEEDBACK_MS } from '../lib/copyFeedback'
import { badgeMarkdown, componentBadgeUrl } from '../lib/statusPageHelpers'
import {
  useStatusComponents,
  type StatusPageSettings,
} from '../queries/statusPage'

const OVERALL = '__overall__'

function CopyRow({
  label,
  value,
  mono = true,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  const { t } = useTranslation('statusPage')
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-center gap-2">
      <span className="w-28 shrink-0 text-xs text-muted-foreground">
        {label}
      </span>
      <Input
        readOnly
        value={value}
        aria-label={label}
        className={mono ? 'h-8 font-mono text-xs' : 'h-8 text-xs'}
        onFocus={(e) => {
          e.currentTarget.select()
        }}
      />
      <Button
        type="button"
        size="icon-sm"
        variant="outline"
        aria-label={t('links.copy', { what: label })}
        onClick={() => {
          void navigator.clipboard.writeText(value).then(() => {
            setCopied(true)
            window.setTimeout(() => {
              setCopied(false)
            }, COPY_FEEDBACK_MS)
          })
        }}
      >
        {copied ? (
          <CheckGlassIcon className="size-5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-5" aria-hidden="true" />
        )}
        <span className="sr-only" aria-live="polite">
          {copied ? t('links.copied') : ''}
        </span>
      </Button>
    </div>
  )
}

export function StatusPublicLinks({
  settings,
}: {
  settings: StatusPageSettings
}) {
  const { t } = useTranslation('statusPage')
  const { data: components } = useStatusComponents()
  const [target, setTarget] = useState(OVERALL)
  const origin = settings.custom_domain
    ? `https://${settings.custom_domain}`
    : window.location.origin
  // A custom domain serves the page at its root; the control plane host under /public.
  const prefix = settings.custom_domain ? origin : `${origin}/public`
  const pageUrl = settings.custom_domain
    ? `${origin}/`
    : `${origin}${settings.public_path ?? '/public/status'}`
  const badgeBase = `${prefix}/status/badge.svg`
  const feed = settings.custom_domain
    ? `${origin}/status.atom`
    : `${origin}/public/status.atom`
  const badgeUrl =
    target === OVERALL ? badgeBase : componentBadgeUrl(badgeBase, target)
  const picked = (components ?? []).find((c) => c.id === target)
  const label = picked ? picked.display_name : settings.title || 'status'

  return (
    <section className="space-y-2" aria-labelledby="sp-links-title">
      <h2 id="sp-links-title" className="text-sm font-semibold text-foreground">
        {t('links.title')}
      </h2>
      <CopyRow label={t('links.page')} value={pageUrl} />
      <CopyRow label={t('links.atom')} value={feed} />
      <CopyRow label={t('links.history')} value={`${prefix}/status/history`} />
      <CopyRow label={t('links.badge')} value={badgeUrl} />
      <div className="flex items-center gap-2">
        <span className="w-28 shrink-0 text-xs text-muted-foreground">
          {t('links.badgeFor')}
        </span>
        <Select
          value={target}
          onValueChange={(v) => {
            setTarget(typeof v === 'string' ? v : OVERALL)
          }}
        >
          <SelectTrigger
            className="w-full max-w-xs"
            aria-label={t('links.badgeFor')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={OVERALL}>{t('links.badgeOverall')}</SelectItem>
            {(components ?? [])
              .filter((c) => c.id)
              .map((c) => (
                <SelectItem key={c.id} value={c.id ?? ''}>
                  {c.display_name}
                </SelectItem>
              ))}
          </SelectContent>
        </Select>
      </div>
      <CopyRow
        label={t('links.markdown')}
        value={badgeMarkdown(pageUrl, badgeUrl, label)}
      />
    </section>
  )
}
