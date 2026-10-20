import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyIcon, PlugsIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useReverseProxyGuide } from '../queries/reverseProxyGuide'

const PROXIES = ['traefik', 'nginx', 'caddy'] as const

export function ReverseProxyGuideCard() {
  const { t } = useTranslation('domains')
  const guide = useReverseProxyGuide()
  const [domain, setDomain] = useState('')
  const [proxy, setProxy] = useState<string>('')
  const [copied, setCopied] = useState(false)
  const data = guide.data
  const plan = data?.plan

  function run(verify: boolean) {
    guide.mutate({ domain: domain.trim(), proxy: proxy || undefined, verify })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <PlugsIcon className="size-4" />
          {t('reverseProxy.title')}
        </CardTitle>
        <CardDescription>{t('reverseProxy.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <div className="space-y-1.5">
            <Label htmlFor="rp-domain">{t('reverseProxy.domain')}</Label>
            <Input
              id="rp-domain"
              value={domain}
              placeholder={t('reverseProxy.domainPlaceholder')}
              onChange={(e) => setDomain(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="rp-proxy">{t('reverseProxy.proxy')}</Label>
            <select
              id="rp-proxy"
              className="h-9 rounded-md border border-input bg-background px-2 text-sm"
              value={proxy}
              onChange={(e) => setProxy(e.target.value)}
            >
              <option value="">{data?.detected_proxy ?? 'auto'}</option>
              {PROXIES.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="flex gap-2">
          <Button
            type="button"
            disabled={guide.isPending || domain.trim() === ''}
            onClick={() => run(false)}
          >
            {t('reverseProxy.build')}
          </Button>
          {plan ? (
            <Button
              type="button"
              variant="outline"
              disabled={guide.isPending}
              onClick={() => run(true)}
            >
              {t('reverseProxy.verify')}
            </Button>
          ) : null}
        </div>
        {data && data.holders.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('reverseProxy.none')}
          </p>
        ) : null}
        {data?.holders.map((h) => (
          <p key={`${h.port}-${h.container}`} className="text-sm">
            {t('reverseProxy.holder', {
              port: h.port,
              container: h.container,
              image: h.image,
            })}
          </p>
        ))}
        {plan ? (
          <div className="space-y-3">
            <ol className="list-decimal space-y-1 pl-5 text-sm">
              {plan.steps.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
            <div className="relative">
              <pre className="overflow-x-auto rounded-md border border-border bg-muted/40 p-3 text-xs">
                {plan.snippet}
              </pre>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="absolute right-2 top-2"
                onClick={() => {
                  void navigator.clipboard.writeText(plan.snippet).then(() => {
                    setCopied(true)
                  })
                }}
              >
                <CopyIcon className="size-3" />
                {copied ? t('reverseProxy.copied') : t('reverseProxy.copy')}
              </Button>
            </div>
          </div>
        ) : null}
        {data?.check ? (
          <Alert variant={data.check.reachable ? 'default' : 'destructive'}>
            <AlertDescription>
              {data.check.reachable
                ? t('reverseProxy.reachable')
                : t('reverseProxy.notReachable')}{' '}
              {data.check.detail}
            </AlertDescription>
          </Alert>
        ) : null}
        {guide.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('reverseProxy.error')}: {guide.error.message}
            </AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
