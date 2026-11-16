import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { useDeploymentLogs } from '../../queries/clusters'
import type { K8sPod } from '../../types/clusters'

const DEFAULT_TAIL = Number(import.meta.env.VITE_K8S_LOG_TAIL) || 200

export function DeploymentLogsSheet({
  cluster,
  app,
  pods,
  open,
  onOpenChange,
}: Readonly<{
  cluster: string
  app: string
  pods: readonly K8sPod[]
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('clusters')
  const [tail, setTail] = useState(String(DEFAULT_TAIL))
  const [pod, setPod] = useState('')
  const tailNum = Math.max(1, Math.floor(Number(tail)) || DEFAULT_TAIL)
  const logs = useDeploymentLogs(cluster, app, pod, tailNum, open)
  const { copied, copy } = useCopyToClipboard()

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="data-[side=right]:sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>{t('logs.title', { name: app })}</SheetTitle>
          <SheetDescription className="sr-only">
            {t('logs.description')}
          </SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-2 px-4 pb-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="flex flex-col gap-1">
              <Label htmlFor="k8s-log-tail">{t('logs.tail')}</Label>
              <Input
                id="k8s-log-tail"
                type="number"
                min={1}
                className="w-24"
                value={tail}
                onChange={(e) => {
                  setTail(e.target.value)
                }}
              />
            </div>
            {pods.length > 1 ? (
              <div className="flex flex-col gap-1">
                <Label htmlFor="k8s-log-pod">{t('logs.pod')}</Label>
                <select
                  id="k8s-log-pod"
                  value={pod}
                  onChange={(e) => {
                    setPod(e.target.value)
                  }}
                  className="h-9 rounded-md border border-input bg-background px-2 text-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  <option value="">{t('logs.anyPod')}</option>
                  {pods.map((p) => (
                    <option key={p.name} value={p.name}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
            <div className="ml-auto flex gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={logs.isFetching}
                onClick={() => {
                  void logs.refetch()
                }}
              >
                <RefreshCwGlassIcon aria-hidden="true" />
                {t('logs.refresh')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={!logs.data?.text}
                onClick={() => {
                  copy(logs.data?.text ?? '')
                }}
              >
                <CopyGlassIcon aria-hidden="true" />
                {copied ? t('logs.copied') : t('logs.copy')}
              </Button>
            </div>
          </div>
          {logs.isPending ? (
            <Skeleton className="h-40 w-full" />
          ) : logs.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {logs.error.message}
            </p>
          ) : (
            <>
              {logs.data.truncated ? (
                <p className="text-xs text-muted-foreground">
                  {t('logs.truncated')}
                </p>
              ) : null}
              <pre
                tabIndex={0}
                aria-label={t('logs.output')}
                className="min-h-0 flex-1 overflow-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-xs break-words whitespace-pre-wrap focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              >
                {logs.data.text || t('logs.empty')}
              </pre>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
