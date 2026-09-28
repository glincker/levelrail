import { LinkIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { HelpLink } from '@/components/HelpLink'
import { toast } from '@/components/ui/toast'
import { useRebindSecrets, useSecretBinding } from '../queries/secretBinding'

export function SecretBindingCard() {
  const { data: status, isLoading } = useSecretBinding()
  const rebind = useRebindSecrets()

  if (isLoading || !status) {
    return null
  }

  const failures = rebind.data?.failed ?? []

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <LinkIcon className="size-4" />
            </div>
            <div className="min-w-0">
              <CardTitle className="flex items-center gap-1.5">
                Secret slot binding
                <HelpLink
                  path="/master-key-rotation#binding-secrets-to-their-slot"
                  label="Secret slot binding"
                />
              </CardTitle>
              <CardDescription>
                Bound values only decrypt for the app and key they were set on,
                so a copied database row cannot be read elsewhere.
              </CardDescription>
            </div>
          </div>
          {status.legacy > 0 ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={rebind.isPending}
              onClick={() => {
                rebind.mutate(undefined, {
                  onSuccess: (result) => {
                    toast.add({
                      title:
                        result.failedCount > 0
                          ? `Bound ${result.rebound} value(s), ${result.failedCount} could not be bound.`
                          : `Bound ${result.rebound} value(s).`,
                      type: result.failedCount > 0 ? 'error' : 'success',
                    })
                  },
                  onError: (err) => {
                    toast.add({ title: err.message, type: 'error' })
                  },
                })
              }}
            >
              {rebind.isPending ? 'Binding...' : 'Bind now'}
            </Button>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          {status.legacy > 0 ? (
            <Badge variant="warning">{status.legacy} legacy</Badge>
          ) : (
            <Badge variant="success">All bound</Badge>
          )}
          <span className="text-muted-foreground">
            {status.bound} of {status.total} secret values bound
          </span>
        </div>
        {failures.length > 0 ? (
          <ul className="space-y-1 text-xs text-muted-foreground">
            {failures.map((f) => (
              <li key={`${f.owner}/${f.key}`} className="break-all font-mono">
                {f.owner}/{f.key}: {f.reason}
              </li>
            ))}
          </ul>
        ) : null}
      </CardContent>
    </Card>
  )
}
