import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  DownloadSimpleIcon,
  ExportIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import {
  useLoadBalancerExport,
  type LoadBalancerExportFormat,
} from '../queries/appLoadBalancer'

const FORMATS: { value: LoadBalancerExportFormat; label: string }[] = [
  { value: 'terraform', label: 'Terraform' },
  { value: 'cdk', label: 'AWS CDK' },
  { value: 'cloudformation', label: 'CloudFormation' },
  { value: 'caddy', label: 'Caddyfile' },
  { value: 'caddy-json', label: 'Caddy JSON' },
]

function downloadText(filename: string, body: string) {
  const url = URL.createObjectURL(new Blob([body], { type: 'text/plain' }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

function ExportPreview({
  appName,
  format,
}: {
  appName: string
  format: LoadBalancerExportFormat
}) {
  const artifact = useLoadBalancerExport(appName, format, true)
  const { copied, copy } = useCopyToClipboard()

  if (artifact.isLoading) return <Skeleton className="h-48 w-full" />
  if (artifact.isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {artifact.error.message}
      </p>
    )
  }
  const data = artifact.data
  if (!data) return null

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <code className="text-xs text-muted-foreground">{data.filename}</code>
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => copy(data.body)}
          >
            {copied ? (
              <CheckIcon data-icon="inline-start" />
            ) : (
              <CopyIcon data-icon="inline-start" />
            )}
            {copied ? 'Copied' : 'Copy'}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => downloadText(data.filename, data.body)}
          >
            <DownloadSimpleIcon data-icon="inline-start" />
            Download
          </Button>
        </div>
      </div>
      <pre
        aria-label={`${data.filename} contents`}
        className="max-h-96 overflow-auto rounded-lg border bg-muted/40 p-3 text-xs"
      >
        {data.body}
      </pre>
      {data.warnings && data.warnings.length > 0 ? (
        <ul className="list-disc space-y-1 pl-5 text-xs text-amber-700 dark:text-amber-300">
          {data.warnings.map((w) => (
            <li key={w}>{w}</li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

export function LoadBalancerExportDialog({ appName }: { appName: string }) {
  const [format, setFormat] = useState<LoadBalancerExportFormat>('terraform')
  const [open, setOpen] = useState(false)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <ExportIcon data-icon="inline-start" />
        Export
      </DialogTrigger>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Export load balancer</DialogTitle>
          <DialogDescription>
            Generates a definition from the saved config. Nothing is sent to a
            cloud provider.
          </DialogDescription>
        </DialogHeader>
        <Tabs
          value={format}
          onValueChange={(value) =>
            setFormat(value as LoadBalancerExportFormat)
          }
        >
          <TabsList>
            {FORMATS.map((f) => (
              <TabsTrigger key={f.value} value={f.value}>
                {f.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {open ? <ExportPreview appName={appName} format={format} /> : null}
      </DialogContent>
    </Dialog>
  )
}
