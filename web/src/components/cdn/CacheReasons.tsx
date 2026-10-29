import { useTranslation } from 'react-i18next'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { NEXTJS_VARY, canVaryOn } from '../../lib/cachePresets'
import {
  useFixCacheVary,
  type CacheReason,
  type CacheReasonCode,
} from '../../queries/cdnCache'
import { formatCount } from './format'

const KNOWN: CacheReasonCode[] = [
  'vary_header',
  'set_cookie',
  'private',
  'no_store',
  'no_cache',
  'status_not_cached',
  'too_large',
  'range',
  'authorization',
  'cookie_request',
  'upgrade',
]

function codeKey(code: CacheReasonCode): CacheReasonCode {
  return KNOWN.includes(code) ? code : 'other'
}

/** The leading reason as short text, for the hit rate cell. */
export function DominantReason({
  reasons,
}: Readonly<{ reasons: CacheReason[] }>) {
  const { t } = useTranslation('cdn')
  const top = reasons[0]
  if (!top) return null
  return (
    <span className="text-xs text-muted-foreground">
      {t(`reasons.short.${codeKey(top.code)}`, { header: top.detail ?? '' })}
    </span>
  )
}

/** Why responses for an app stay out of the cache, with one-click fixes. */
export function CacheReasons({
  app,
  reasons,
  suggestPreset,
}: Readonly<{
  app: string
  reasons: CacheReason[]
  suggestPreset?: string
}>) {
  const { t } = useTranslation('cdn')
  const fix = useFixCacheVary(app)
  if (reasons.length === 0) {
    return <span className="text-muted-foreground">{t('reasons.none')}</span>
  }
  const nextjs = suggestPreset === 'nextjs'
  const nextHeaders = new Set(NEXTJS_VARY)
  const varyReasons = reasons.filter((r) => r.code === 'vary_header')
  const presetDomains = [...new Set(varyReasons.flatMap((r) => r.domains))]
  const inPreset = (r: CacheReason) =>
    nextjs && nextHeaders.has((r.detail ?? '').toLowerCase())
  return (
    <details className="group max-w-md">
      <summary className="cursor-pointer text-sm underline-offset-4 hover:underline">
        <DominantReason reasons={reasons} />
      </summary>
      <ul className="mt-2 space-y-2">
        {reasons.map((r) => {
          const key = `${r.code}:${r.detail ?? ''}`
          const header = r.detail ?? ''
          const canFix =
            r.code === 'vary_header' &&
            canVaryOn(header) &&
            !inPreset(r) &&
            r.domains.length > 0
          return (
            <li key={key} className="flex flex-wrap items-center gap-2 text-sm">
              <span className="flex-1">
                <span className="tabular-nums font-medium">
                  {formatCount(r.count)}
                </span>{' '}
                {t(`reasons.long.${codeKey(r.code)}`, { header })}
              </span>
              {canFix ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  writeAction
                  disabled={fix.isPending}
                  onClick={() =>
                    fix.mutate({ domains: r.domains, headers: [header] })
                  }
                >
                  <WrenchGlassIcon size="1.2em" aria-hidden="true" />
                  {t('reasons.fixVary', { header })}
                </Button>
              ) : null}
            </li>
          )
        })}
      </ul>
      {nextjs && presetDomains.length > 0 ? (
        <div className="mt-2 flex flex-wrap items-center gap-2 rounded-lg bg-muted/40 px-3 py-2 text-sm">
          <span className="min-w-40 flex-1">{t('reasons.nextjsHelp')}</span>
          <Button
            type="button"
            size="sm"
            writeAction
            disabled={fix.isPending}
            onClick={() =>
              fix.mutate({
                domains: presetDomains,
                headers: NEXTJS_VARY,
                nextjs: true,
              })
            }
          >
            <WrenchGlassIcon size="1.2em" aria-hidden="true" />
            {t('reasons.nextjsFix')}
          </Button>
        </div>
      ) : null}
      {fix.error ? (
        <p role="alert" className="mt-2 text-sm text-destructive">
          {fix.error.message}
        </p>
      ) : null}
    </details>
  )
}
