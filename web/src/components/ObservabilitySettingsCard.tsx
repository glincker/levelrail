import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Link } from '@tanstack/react-router'
import {
  CheckIcon,
  ChartLineIcon,
  CopyIcon,
  LinkSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { ObservabilitySettings } from '../queries/observabilitySettings'
import { useUpdateObservabilitySettings } from '../queries/observabilitySettings'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

// Read-only, server-generated: the real address this build serves,
// mirroring DatabasePublicAccessCard's copy-row idiom (code block + a
// Copy button that flips to a checkmark for 1 render, no toast needed
// since the row itself is the confirmation).
export function RemoteReadConnectionCard({
  settings,
}: {
  settings: ObservabilitySettings
}) {
  const [copied, setCopied] = useState(false)
  const remoteReadURL = `${window.location.origin}${settings.remote_read_path}`

  function copyURL() {
    void navigator.clipboard.writeText(remoteReadURL).then(() => {
      setCopied(true)
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ChartLineIcon className="size-4" />
          Remote read (Prometheus)
        </CardTitle>
        <CardDescription>
          This control plane already answers Prometheus remote-read queries, so
          any Grafana instance you run, cloud-hosted or on your own network, can
          add it as a Prometheus data source.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Field>
          <FieldLabel htmlFor="remote-read-url">Remote read URL</FieldLabel>
          <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
            <code
              id="remote-read-url"
              className="min-w-0 flex-1 overflow-x-auto text-xs break-all"
            >
              {remoteReadURL}
            </code>
            <Button type="button" size="sm" variant="outline" onClick={copyURL}>
              {copied ? <CheckIcon /> : <CopyIcon />}
              {copied ? 'Copied' : 'Copy'}
            </Button>
          </div>
          <FieldDescription>
            In Grafana, add a Prometheus data source with this as the URL, query
            type &quot;remote read&quot;.
          </FieldDescription>
        </Field>
        <Alert>
          <AlertDescription>
            This endpoint requires an API token scoped to at least{' '}
            <code>read</code>. Create one on the{' '}
            <Link
              to="/settings/tokens"
              className="underline underline-offset-4"
            >
              Tokens
            </Link>{' '}
            page and set it as Grafana&apos;s data source authorization header
            (Bearer). No token means this build has no auth gate on its own
            beyond that: don&apos;t expose it to an untrusted network without
            one.
          </AlertDescription>
        </Alert>
      </CardContent>
    </Card>
  )
}

const observabilitySettingsSchema = z.object({
  externalDashboardUrl: z
    .string()
    .trim()
    .refine(
      (v) => v === '' || /^https?:\/\/.+/.test(v),
      'Must be an absolute http:// or https:// URL',
    ),
})

type ObservabilitySettingsFormValues = z.infer<
  typeof observabilitySettingsSchema
>

// The external dashboard link: a plain URL an operator sets once,
// pointing at wherever they already run Grafana. Plain link only, no
// embedding, no iframe, no SSO proxying, consumed by ViewInGrafanaLink
// next to the existing metrics charts on app/node detail pages.
export function ObservabilitySettingsCard({
  settings,
}: {
  settings: ObservabilitySettings
}) {
  const updateSettings = useUpdateObservabilitySettings()
  const { register, handleSubmit, formState } =
    useForm<ObservabilitySettingsFormValues>({
      resolver: zodResolver(observabilitySettingsSchema),
      values: { externalDashboardUrl: settings.external_dashboard_url },
    })

  const onSubmit = handleSubmit((values) => {
    updateSettings.mutate(
      { external_dashboard_url: values.externalDashboardUrl },
      {
        onSuccess: () => {
          toast.add({ title: 'Observability settings saved.', type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LinkSimpleIcon className="size-4" />
          External dashboard
        </CardTitle>
        <CardDescription>
          Where your own Grafana (or other dashboard) lives. Once set, a
          &quot;View in Grafana&quot; link appears next to the metrics charts on
          app and node detail pages. This is a plain link, nothing is embedded
          here.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          // The input's own type="url" otherwise lets the browser's native
          // constraint validation block submission before zod ever runs,
          // showing its generic "enter a URL" tooltip instead of this
          // form's own, more specific error message.
          noValidate
          className="space-y-5"
        >
          <Field>
            <FieldLabel htmlFor="external-dashboard-url">
              Dashboard URL
            </FieldLabel>
            <Input
              id="external-dashboard-url"
              type="url"
              placeholder="https://grafana.example.internal"
              {...register('externalDashboardUrl')}
            />
            <FieldError errors={[formState.errors.externalDashboardUrl]} />
            <FieldDescription>
              Leave blank to hide the link. Points at an instance you run
              yourself, Grafana Cloud or self-hosted on your own network.
            </FieldDescription>
          </Field>

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateSettings.isPending}>
              {updateSettings.isPending ? 'Saving...' : 'Save'}
            </Button>
          </div>
          {updateSettings.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {updateSettings.error.message}
              </AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
