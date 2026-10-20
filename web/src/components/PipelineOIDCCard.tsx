import { useState } from 'react'
import { CheckIcon, CopyIcon, KeyIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { HelpLink } from '@/components/HelpLink'
import { usePipelineOIDCInfo } from '../queries/pipelineOidc'
import { RotatePipelineOIDCKeyDialog } from './RotatePipelineOIDCKeyDialog'

// Surfaces GET /api/v1/pipelines/oidc so a job's `oidc: {audience: ...}`
// config (docs/pipelines.md#cloud-credentials-via-oidc) has a visible,
// copyable URL to wire into a cloud provider, not just a YAML field
// nobody can discover from the dashboard. Renders nothing while
// loading, on error, or when OIDC isn't configured on this control
// plane: an info banner would be noise on a page most operators never
// need this for.
export function PipelineOIDCCard() {
  const { data } = usePipelineOIDCInfo()
  const [copied, setCopied] = useState(false)

  if (!data?.configured || !data.jwks_url) {
    return null
  }

  return (
    <Alert>
      <KeyIcon className="size-4" aria-hidden="true" />
      <AlertDescription className="space-y-2">
        <p>
          Jobs can mint OIDC tokens for cloud provider federation (AWS IAM, GCP
          workload identity, Vault). Wire this JWKS URL to the provider's trust
          policy.{' '}
          <HelpLink
            path="/pipelines-oidc"
            label="Setup guide"
            variant="inline"
          />
        </p>
        <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
          <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
            {data.jwks_url}
          </code>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void navigator.clipboard
                .writeText(data.jwks_url ?? '')
                .then(() => {
                  setCopied(true)
                })
            }}
          >
            {copied ? <CheckIcon /> : <CopyIcon />}
            {copied ? 'Copied' : 'Copy'}
          </Button>
        </div>
        {data.rotation_supported ? (
          <div>
            <RotatePipelineOIDCKeyDialog />
          </div>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
