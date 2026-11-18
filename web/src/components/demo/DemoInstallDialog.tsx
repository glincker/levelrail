import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { useDemoInfo } from '@/hooks/useDemo'
import { captureDemoEvent } from '@/lib/demoAnalytics'
import { sendDemoCta } from '@/queries/demo'
import { DEFAULT_REPO_URL } from './DemoBanner'

const STEPS = ['run', 'open', 'deploy'] as const

/** DemoInstallDialog shows the one-line install command, three steps and the docs and GitHub links. */
export function DemoInstallDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const { copied, copy } = useCopyToClipboard()
  const command = info.installCommand ?? ''

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('install.title')}</DialogTitle>
          <DialogDescription>{t('install.description')}</DialogDescription>
        </DialogHeader>
        {command ? (
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
            <code className="min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-nowrap">
              {command}
            </code>
            <Button
              type="button"
              size="xs"
              variant="outline"
              onClick={() => {
                copy(command)
                sendDemoCta('install')
                captureDemoEvent('demo_install_command_copied')
              }}
            >
              {copied ? (
                <CheckGlassIcon size="1.4em" data-icon="inline-start" />
              ) : (
                <CopyGlassIcon size="1.4em" data-icon="inline-start" />
              )}
              {copied ? t('banner.copied') : t('install.copy')}
            </Button>
          </div>
        ) : null}
        <ol className="list-decimal space-y-1 pl-5 text-sm">
          {STEPS.map((s) => (
            <li key={s}>{t(`install.steps.${s}`)}</li>
          ))}
        </ol>
        <DialogFooter>
          {info.docsUrl ? (
            <Button
              variant="outline"
              size="sm"
              nativeButton={false}
              render={
                <a
                  href={info.docsUrl}
                  target="_blank"
                  rel="noopener"
                  onClick={() => {
                    sendDemoCta('docs')
                  }}
                />
              }
            >
              {t('banner.docs')}
            </Button>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={
              <a
                href={info.repoUrl || DEFAULT_REPO_URL}
                target="_blank"
                rel="noopener"
                onClick={() => {
                  sendDemoCta('github')
                  captureDemoEvent('demo_github_click')
                }}
              />
            }
          >
            {t('banner.github')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
