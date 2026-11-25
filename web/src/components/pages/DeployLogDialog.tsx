import { useQuery } from '@tanstack/react-query'
import { Fragment, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, DownloadGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { pagesDeployLogQueryOptions } from '../../queries/pagesDeployLog'
import DeployFailHint from './DeployFailHint'

/** Build log of one Pages deploy. Loaded lazily by RecentDeploys. */
export default function DeployLogDialog({
  name,
  deployId,
  open,
  onOpenChange,
}: Readonly<{
  name: string
  deployId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('pagesSites')
  const { data, isPending, isError } = useQuery({
    ...pagesDeployLogQueryOptions(name, deployId),
    enabled: open,
  })
  const endRef = useRef<HTMLDivElement>(null)
  const [copied, setCopied] = useState(false)
  const lines = data?.lines ?? []
  const text = lines.map((l) => l.line).join('\n')

  useEffect(() => {
    if (data?.running) endRef.current?.scrollIntoView({ block: 'end' })
  }, [data?.running, lines.length])

  const download = () => {
    const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${name}-${deployId}.log`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="rich">
        <DialogHeader>
          <DialogTitle>{t('logs.title')}</DialogTitle>
        </DialogHeader>
        <div className="flex gap-1">
          <Button
            size="xs"
            variant="ghost"
            aria-label={copied ? t('logs.copied') : t('logs.copy')}
            title={copied ? t('logs.copied') : t('logs.copy')}
            onClick={() => {
              void navigator.clipboard.writeText(text).then(() => {
                setCopied(true)
                setTimeout(() => setCopied(false), 1500)
              })
            }}
          >
            <CopyGlassIcon aria-hidden="true" />
          </Button>
          <Button
            size="xs"
            variant="ghost"
            aria-label={t('logs.download')}
            title={t('logs.download')}
            onClick={download}
          >
            <DownloadGlassIcon aria-hidden="true" />
          </Button>
        </div>
        <div
          role="log"
          aria-live="off"
          className="max-h-[60vh] overflow-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-xs"
        >
          {isPending ? <p>{t('logs.loading')}</p> : null}
          {isError ? (
            <p className="text-destructive">{t('logs.error')}</p>
          ) : null}
          {!isPending && !isError && lines.length === 0 ? (
            <p className="text-muted-foreground">{t('logs.empty')}</p>
          ) : null}
          {lines.map((l, i) => (
            <Fragment key={`${l.at ?? ''}-${i}`}>
              {l.step && l.step !== lines[i - 1]?.step ? (
                <h3 className="mt-2 font-semibold first:mt-0">
                  {t(`logs.step.${l.step}`, { defaultValue: l.step })}
                </h3>
              ) : null}
              <div
                className={`break-all whitespace-pre-wrap ${l.stream === 'stderr' ? 'text-destructive' : ''}`}
              >
                {l.line}
              </div>
            </Fragment>
          ))}
          <div ref={endRef} />
        </div>
        {data?.status === 'failed' ? (
          <DeployFailHint lines={lines.map((l) => l.line)} />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
