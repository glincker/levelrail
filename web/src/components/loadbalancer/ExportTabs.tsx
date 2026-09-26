import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  DownloadSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { SkeletonLine } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  useLoadBalancerExport,
  type LoadBalancerExportFormat,
} from '../../queries/appLoadBalancer'
import { TOKEN_CLASS, tokenize } from './highlight'

const FORMATS: {
  value: LoadBalancerExportFormat
  label: string
  apply: (file: string, app: string) => string
}[] = [
  { value: 'terraform', label: 'Terraform', apply: () => 'terraform apply' },
  { value: 'cdk', label: 'CDK', apply: () => 'cdk deploy' },
  {
    value: 'cloudformation',
    label: 'CloudFormation',
    apply: (f, app) =>
      `aws cloudformation deploy --template-file ${f} --stack-name ${app}-lb`,
  },
  {
    value: 'caddy',
    label: 'Caddy',
    apply: (f) => `caddy reload --config ${f}`,
  },
  {
    value: 'caddy-json',
    label: 'Caddy JSON',
    apply: (f) =>
      `curl -X POST localhost:2019/load -H 'Content-Type: application/json' -d @${f}`,
  },
]

function download(filename: string, body: string) {
  const url = URL.createObjectURL(new Blob([body], { type: 'text/plain' }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

function Preview({
  appName,
  format,
}: {
  appName: string
  format: (typeof FORMATS)[number]
}) {
  const artifact = useLoadBalancerExport(appName, format.value, true)
  const { copied, copy } = useCopyToClipboard()
  if (artifact.isLoading) {
    return (
      <div className="space-y-2">
        <SkeletonLine width="60%" />
        <SkeletonLine width="80%" />
        <SkeletonLine width="45%" />
      </div>
    )
  }
  if (artifact.isError) {
    return (
      <p role="alert" className="text-sm text-tone-danger">
        {artifact.error.message}
      </p>
    )
  }
  const data = artifact.data
  if (!data) return null
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
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
            onClick={() => download(data.filename, data.body)}
          >
            <DownloadSimpleIcon data-icon="inline-start" />
            Download
          </Button>
        </div>
      </div>
      <pre
        aria-label={`${data.filename} contents`}
        className="max-h-96 overflow-auto rounded-xl border bg-muted/40 p-3 font-mono text-xs leading-relaxed"
      >
        <code>
          {tokenize(data.body).map((t, i) => (
            <span key={i} className={TOKEN_CLASS[t.kind]}>
              {t.text}
            </span>
          ))}
        </code>
      </pre>
      <p className="text-xs text-muted-foreground">
        Apply with{' '}
        <code className="rounded bg-muted px-1.5 py-0.5">
          {format.apply(data.filename, appName)}
        </code>
      </p>
      {data.warnings && data.warnings.length > 0 ? (
        <ul className="list-disc space-y-1 pl-5 text-xs text-tone-warning">
          {data.warnings.map((w) => (
            <li key={w}>{w}</li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

export function ExportTabs({ appName }: { appName: string }) {
  const [value, setValue] = useState<LoadBalancerExportFormat>('terraform')
  const format = FORMATS.find((f) => f.value === value) ?? FORMATS[0]
  return (
    <div className="space-y-3">
      <Tabs
        value={value}
        onValueChange={(v) => setValue(v as LoadBalancerExportFormat)}
      >
        <TabsList className="max-w-full overflow-x-auto">
          {FORMATS.map((f) => (
            <TabsTrigger key={f.value} value={f.value}>
              {f.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      {format ? <Preview appName={appName} format={format} /> : null}
    </div>
  )
}
