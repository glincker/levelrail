import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { BrandIcon } from '@/components/BrandIcon'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useDnsProviderPlugin,
  useSetDnsProviderPlugin,
  useTestDnsPlugin,
} from '../../queries/pluginDns'
import type { Plugin } from '../../queries/plugins'

// Test and choose a dns_provider plugin. Only shown once the plugin runs,
// because both actions are real calls into it.
export function DnsProviderControls({ plugin }: Readonly<{ plugin: Plugin }>) {
  const { t } = useTranslation('plugins')
  const setting = useDnsProviderPlugin()
  const choose = useSetDnsProviderPlugin()
  const test = useTestDnsPlugin()
  const active = setting.data?.plugin === plugin.name
  const ready = plugin.state === 'enabled' && plugin.running

  const fail = (error: unknown) =>
    toast.add({
      type: 'error',
      title: t('actions.failed'),
      description: error instanceof Error ? error.message : undefined,
    })

  return (
    <div className="mt-2 space-y-1.5 text-xs">
      <p className="flex items-center gap-1.5 text-muted-foreground">
        <BrandIcon name="cloudflare" className="size-4" />
        {active ? t('dns.active') : t('dns.inactive')}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!ready || test.isPending}
          onClick={() => test.mutate(plugin.name, { onError: fail })}
        >
          <PlugGlassIcon size="1.4em" aria-hidden="true" />
          {test.isPending ? t('dns.testing') : t('dns.test')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant={active ? 'outline' : 'default'}
          disabled={!ready || choose.isPending}
          onClick={() =>
            choose.mutate(active ? '' : plugin.name, {
              onSuccess: () =>
                toast.add({
                  type: 'success',
                  title: active
                    ? t('dns.builtInOn')
                    : t('dns.pluginOn', { name: plugin.name }),
                }),
              onError: fail,
            })
          }
        >
          {active ? t('dns.useBuiltIn') : t('dns.use')}
        </Button>
      </div>
      {test.data ? (
        <p
          role="status"
          className={
            test.data.ok ? 'text-muted-foreground' : 'text-destructive'
          }
        >
          {test.data.ok
            ? t('dns.testOk', { count: test.data.zones?.length ?? 0 })
            : t('dns.testFailed', { message: test.data.message })}
        </p>
      ) : null}
      {ready ? null : (
        <p className="text-muted-foreground">{t('dns.needsRunning')}</p>
      )}
    </div>
  )
}
