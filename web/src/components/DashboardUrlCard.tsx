import * as React from 'react'
import { LockKeyIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useDashboardUrl, useUpdateDashboardUrl } from '../queries/dashboardUrl'
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
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

export const DASHBOARD_URL_ANCHOR = 'dashboard-url'

// The public URL operators use to reach the dashboard. Once it is https,
// the server refuses sign-in over plain HTTP.
export function DashboardUrlCard() {
  const { data } = useDashboardUrl()
  const update = useUpdateDashboardUrl()
  const [draft, setDraft] = React.useState<string | null>(null)
  const value = draft ?? data.dashboard_url

  return (
    <Card id={DASHBOARD_URL_ANCHOR} className="scroll-mt-20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockKeyIcon className="size-4" />
          Dashboard URL
        </CardTitle>
        <CardDescription>
          Where operators reach this dashboard. Set it to an https address (for
          example the primary domain above) to stop sign-in over plain HTTP.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            update.mutate(
              { dashboard_url: value.trim() },
              {
                onSuccess: () => {
                  setDraft(null)
                  toast.add({ title: 'Dashboard URL saved.', type: 'success' })
                },
              },
            )
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="dashboard-url-input">
                Public dashboard URL
              </FieldLabel>
              <Input
                id="dashboard-url-input"
                className="font-mono"
                placeholder="https://dashboard.example.com"
                value={value}
                onChange={(e) => setDraft(e.target.value)}
              />
              <FieldDescription>
                Save an https URL from a browser tab already open on that URL,
                so it is proven to work. Recovery if it breaks: set
                APP_ALLOW_INSECURE_LOGIN=true on the control plane and restart
                it.
              </FieldDescription>
            </Field>
          </FieldGroup>
          {update.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{update.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <Button type="submit" disabled={update.isPending || draft === null}>
            {update.isPending ? 'Saving...' : 'Save'}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
