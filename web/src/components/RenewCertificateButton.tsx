import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRenewDomainCertificate } from '../queries/domainTlsCert'

// One-click "Renew now" for a single domain's automatically-managed
// certificate (ACME or internal): POST .../cert/renew clears the stored
// certificate and nudges the reconcile loop, which re-issues it within
// seconds rather than waiting for the normal resync interval. No confirm
// dialog, matching RegistryCredentialTable's own TestButton shape: this
// clears cached state, it never deletes an app or loses data, so a plain
// button with toast feedback is enough.
export function RenewCertificateButton({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const renew = useRenewDomainCertificate(appName, domain)

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={renew.isPending}
      title="Clear the stored certificate and request a fresh one on the next reconcile pass."
      onClick={() => {
        renew.mutate(undefined, {
          onSuccess: (result) => {
            toast.add({
              title: `Renewal requested for ${domain}.`,
              description: result.had_stored_certificate
                ? 'The current certificate was cleared; a new one is issued on the next reconcile pass.'
                : 'No certificate was on file yet; one is issued on the next reconcile pass.',
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: `Could not renew the certificate for ${domain}.`,
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
      {renew.isPending ? 'Renewing...' : 'Renew now'}
    </Button>
  )
}
