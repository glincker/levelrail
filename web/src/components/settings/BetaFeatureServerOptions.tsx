import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { RestartControlPlaneButton } from '@/components/RestartControlPlaneButton'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useBrand } from '@/hooks/useBrand'
import { useIsRoot } from '@/hooks/useIsRoot'
import { cliCommand } from '@/lib/cliCommand'
import { useSystemRestartInfo } from '@/queries/systemRestart'
import type { BetaFeature } from '../../types/resourceAdvisor'

const ENV_VAR = 'APP_EXPERIMENTAL'

function CopyLine({ line, label }: { line: string; label: string }) {
  const { t } = useTranslation('betaFeatures')
  const [copied, setCopied] = useState(false)

  function copy() {
    void navigator.clipboard.writeText(line).then(
      () => {
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1500)
      },
      () => undefined,
    )
  }

  return (
    <div className="flex items-center gap-2">
      <code
        aria-label={label}
        className="min-w-0 flex-1 overflow-x-auto rounded-md bg-muted px-2 py-1 font-mono text-xs"
      >
        {line}
      </code>
      <Button type="button" variant="outline" size="sm" onClick={copy}>
        {copied ? (
          <CheckGlassIcon size="1.2em" aria-hidden="true" />
        ) : (
          <CopyGlassIcon size="1.2em" aria-hidden="true" />
        )}
        {copied ? t('env.copied') : t('env.copy')}
      </Button>
    </div>
  )
}

function ServerStep({
  title,
  lines,
}: {
  title: string
  lines: { label: string; line: string }[]
}) {
  return (
    <details className="rounded-md border p-2 text-sm">
      <summary className="cursor-pointer font-medium">{title}</summary>
      <div className="mt-2 space-y-2">
        {lines.map((l) => (
          <CopyLine key={l.line} line={l.line} label={l.label} />
        ))}
      </div>
    </details>
  )
}

export function BetaFeatureServerOptions({
  feature,
  open,
  onOpenChange,
}: {
  feature: BetaFeature
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('betaFeatures')
  const brand = useBrand()
  const isRoot = useIsRoot()
  const { data: info } = useSystemRestartInfo()
  const setting = `${ENV_VAR}=${feature.id}`
  const cliSet = cliCommand(
    brand.BinaryName,
    `settings beta-features set ${feature.id} on`,
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('options.title', { name: feature.name })}
          </DialogTitle>
          <DialogDescription>{t('options.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t('options.cli')}</h3>
            <CopyLine line={cliSet} label={t('options.cliLine')} />
            {feature.requires_restart ? (
              <CopyLine
                line={`${cliSet} --restart`}
                label={t('options.cliRestartLine')}
              />
            ) : null}
          </section>
          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t('options.server')}</h3>
            <ServerStep
              title={t('env.systemd')}
              lines={[
                { label: t('env.line'), line: `Environment=${setting}` },
                { label: t('options.reload'), line: 'systemctl daemon-reload' },
                {
                  label: t('options.restartUnit'),
                  line: `systemctl restart ${brand.ShortName}`,
                },
              ]}
            />
            <ServerStep
              title={t('env.docker')}
              lines={[
                { label: t('env.line'), line: `-e ${setting}` },
                {
                  label: t('options.recreate'),
                  line: 'docker compose up -d --force-recreate',
                },
              ]}
            />
          </section>
          {feature.requires_restart ? (
            <section className="space-y-2">
              <p className="text-sm text-muted-foreground">
                {t('options.restartNote')}
              </p>
              {isRoot && info?.supported ? (
                <RestartControlPlaneButton
                  variant="default"
                  label={t('restart.now')}
                />
              ) : null}
            </section>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
