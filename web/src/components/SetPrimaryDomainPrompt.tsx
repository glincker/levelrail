import { useState } from 'react'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useIngressSettings, useUpdateIngressSettings } from '../queries/domains'

// Shown in place of a provider connect action (GitHubAppConnectionCard,
// GitLabAppConnectionCard, BitbucketAppConnectionCard) whenever
// ingress_settings.primary_domain is unset: every provider's own OAuth
// redirect/manifest callback is built from controlPlaneBaseURL
// (internal/api/control_plane_base_url.go), which requires one. Rather
// than only linking out to full domain settings and leaving the
// operator to find their way back, this saves primary_domain directly
// (PUT /api/v1/settings/ingress), so the connect flow becomes available
// immediately in place, no navigation required. The link to full domain
// settings stays as a secondary path, for ACME email/directory options
// this prompt deliberately doesn't touch.
export function SetPrimaryDomainPrompt() {
  const { data: ingressSettings } = useIngressSettings()
  const update = useUpdateIngressSettings()
  const [domain, setDomain] = useState('')

  function handleSave() {
    const trimmed = domain.trim()
    if (trimmed === '') {
      return
    }
    update.mutate(
      { ...ingressSettings, primary_domain: trimmed },
      {
        onSuccess: () => {
          toast.add({ title: 'Primary domain set.', type: 'success' })
          setDomain('')
        },
        onError: (error) => {
          toast.add({
            title: 'Could not set the primary domain.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <div className="space-y-2 rounded-lg border border-amber-300/60 bg-amber-50 p-3 dark:border-amber-800/60 dark:bg-amber-950/30">
      <p className="flex items-start gap-1.5 text-sm text-amber-800 dark:text-amber-300">
        <WarningIcon className="mt-0.5 size-3.5 shrink-0" />
        <span>
          This instance needs a primary domain before connecting a git
          provider: it&apos;s what OAuth callbacks and manifest URLs point at.
          Set it here, or in{' '}
          <Link to="/domains" className="underline">
            domain settings
          </Link>{' '}
          for ACME/email options too.
        </span>
      </p>
      <div className="flex items-center gap-2">
        <Input
          className="h-8 font-mono text-sm"
          autoComplete="off"
          spellCheck={false}
          placeholder="deploy.example.com"
          value={domain}
          onChange={(e) => {
            setDomain(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              handleSave()
            }
          }}
        />
        <Button
          type="button"
          size="sm"
          disabled={domain.trim() === '' || update.isPending}
          onClick={handleSave}
        >
          {update.isPending ? 'Saving...' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
