import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import {
  PlusGlassIcon,
  ServerGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SkeletonList } from './kit/Skeleton'
import { ManualEnrollFields } from './AddNodeDialog'
import { SSHEnrollFields } from './AddNodeWizardSSH'
import { AddNodeMethodList } from './addNode/AddNodeMethodList'
import { ProvisionProgress, StepShell } from './AddNodeWizardSteps'
import {
  useCreateNodeProvision,
  useNodeProviderRegions,
  useNodeProviderSizes,
  useNodeProviders,
  useNodeProvision,
} from '../queries/nodeProvision'

type ProviderId = 'hetzner' | 'digitalocean' | 'aws' | 'azure' | 'gcp'
type Step =
  | 'method'
  | 'region'
  | 'size'
  | 'details'
  | 'confirm'
  | 'progress'
  | 'manual'
  | 'ssh'

const PROVIDER_LABELS: Record<ProviderId, string> = {
  hetzner: 'Hetzner',
  digitalocean: 'DigitalOcean',
  aws: 'AWS',
  azure: 'Azure',
  gcp: 'Google Cloud',
}

// Suggests a node name from the provider and region already picked by
// this point in the wizard, e.g. "hetzner-fsn1". Always starts with the
// provider id (a fixed lowercase word), so the result always satisfies
// the name field's own ^[a-z][a-z0-9-]*$ pattern below.
function nodeNameFrom(provider: ProviderId, region: string): string {
  return `${provider}-${region}`
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

// A wizard, not a single dialog, for this one flow only: creating a real
// cloud VM genuinely has sequential steps (provider, region, size, name,
// confirm, live progress) that a single screen can't collapse the way
// AddNodeDialog's own manual join-token flow does. "I already have a
// server" reuses that exact flow (ManualEnrollFields) as a step here
// instead of a second implementation of it.
export function AddNodeWizard() {
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" />}>
        <PlusGlassIcon size="1.4em" />
        Add node
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <WizardBody
          key={open ? 'open' : 'closed'}
          onClose={() => setOpen(false)}
        />
      </DialogContent>
    </Dialog>
  )
}

function WizardBody({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const [step, setStep] = useState<Step>('method')
  const [provider, setProvider] = useState<ProviderId | null>(null)
  const [region, setRegion] = useState('')
  const [size, setSize] = useState('')
  const [name, setName] = useState('')
  const [role, setRole] = useState<'general' | 'build'>('general')
  const [allowSSHInbound, setAllowSSHInbound] = useState(false)
  const [controlPlaneAddr, setControlPlaneAddr] = useState(
    () => `${window.location.hostname}:9443`,
  )
  const [provisionId, setProvisionId] = useState<string | null>(null)

  const providers = useNodeProviders()
  const regions = useNodeProviderRegions(provider ?? '', step === 'region')
  const sizes = useNodeProviderSizes(provider ?? '', region, step === 'size')
  const createProvision = useCreateNodeProvision()
  const provision = useNodeProvision(step === 'progress' ? provisionId : null)

  if (step === 'manual') {
    return (
      <ManualEnrollFields onDone={onClose} onBack={() => setStep('method')} />
    )
  }

  if (step === 'ssh') {
    return <SSHEnrollFields onDone={onClose} onBack={() => setStep('method')} />
  }

  if (step === 'method') {
    return (
      <>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ServerGlassIcon className="size-5 text-muted-foreground" />
            Add node
          </DialogTitle>
          <DialogDescription>
            Create a server at a cloud provider and enroll it automatically, or
            connect a machine you already have.
          </DialogDescription>
        </DialogHeader>
        <AddNodeMethodList
          connected={
            new Set(
              (providers.data ?? [])
                .filter((x) => x.has_token)
                .map((x) => x.provider),
            )
          }
          loading={providers.isLoading}
          onPickProvider={(p) => {
            setProvider(p)
            setStep('region')
          }}
          onSSH={() => {
            setStep('ssh')
          }}
          onManual={() => {
            setStep('manual')
          }}
        />
      </>
    )
  }

  if (step === 'region') {
    return (
      <StepShell
        title="Choose a region"
        onBack={() => setStep('method')}
        onContinue={() => setStep('size')}
        continueDisabled={!region}
        continueReason="Select a region to continue."
      >
        {regions.isLoading ? (
          <SkeletonList rows={3} />
        ) : regions.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription>{regions.error.message}</AlertDescription>
          </Alert>
        ) : (
          <Select value={region} onValueChange={(v) => setRegion(v ?? '')}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Select a region" />
            </SelectTrigger>
            <SelectContent>
              {(regions.data ?? []).map((r) => (
                <SelectItem key={r.id} value={r.id}>
                  {r.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </StepShell>
    )
  }

  if (step === 'size') {
    return (
      <StepShell
        title="Choose a size"
        onBack={() => setStep('region')}
        onContinue={() => {
          if (!name && provider) {
            setName(nodeNameFrom(provider, region))
          }
          setStep('details')
        }}
        continueDisabled={!size}
        continueReason="Select a size to continue."
      >
        {sizes.isLoading ? (
          <SkeletonList rows={3} />
        ) : sizes.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription>{sizes.error.message}</AlertDescription>
          </Alert>
        ) : (
          <div className="space-y-2">
            {(sizes.data ?? []).map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => setSize(s.id)}
                className={`flex w-full items-center justify-between rounded-lg border p-3 text-left text-sm transition-colors hover:bg-muted/50 ${
                  size === s.id
                    ? 'border-primary bg-primary/5'
                    : 'border-border'
                }`}
              >
                <span>
                  {s.name} &middot; {s.vcpus} vCPU &middot;{' '}
                  {(s.memory_mb / 1024).toFixed(0)} GB RAM &middot; {s.disk_gb}{' '}
                  GB disk
                </span>
                {s.price_monthly ? (
                  <span className="shrink-0 text-muted-foreground">
                    {s.currency === 'USD' ? '$' : ''}
                    {s.price_monthly}
                    {s.currency && s.currency !== 'USD' ? ` ${s.currency}` : ''}
                    /mo
                  </span>
                ) : (
                  <span className="shrink-0 text-xs text-muted-foreground">
                    pricing varies, see provider console
                  </span>
                )}
              </button>
            ))}
          </div>
        )}
      </StepShell>
    )
  }

  if (step === 'details') {
    return (
      <StepShell
        title="Name and role"
        onBack={() => setStep('size')}
        onContinue={() => setStep('confirm')}
        continueDisabled={!/^[a-z][a-z0-9-]*$/.test(name)}
        continueReason="Name must start with a lowercase letter and use only lowercase letters, digits, and hyphens."
      >
        <Field>
          <FieldLabel htmlFor="node-provision-name">Name</FieldLabel>
          <Input
            id="node-provision-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="web-1"
            className="font-mono text-xs"
          />
          <FieldDescription>
            Lowercase letters, digits and hyphens only.
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="node-provision-role">Role</FieldLabel>
          <Select
            value={role}
            onValueChange={(v) =>
              setRole((v as 'general' | 'build') ?? 'general')
            }
          >
            <SelectTrigger id="node-provision-role" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="general">General</SelectItem>
              <SelectItem value="build">Build</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="node-provision-cp-addr">
            Control plane address
          </FieldLabel>
          <Input
            id="node-provision-cp-addr"
            value={controlPlaneAddr}
            onChange={(e) => setControlPlaneAddr(e.target.value)}
            className="font-mono text-xs"
          />
          <FieldDescription>
            Host and port the new server dials to reach this control plane, port
            9443 by default.
          </FieldDescription>
        </Field>
        {provider === 'aws' ? (
          <div className="flex items-center gap-2">
            <Checkbox
              id="node-provision-aws-ssh"
              checked={allowSSHInbound}
              onCheckedChange={(v) => setAllowSSHInbound(v === true)}
            />
            <FieldLabel
              htmlFor="node-provision-aws-ssh"
              className="font-normal"
            >
              Allow SSH inbound (opt-in; off matches this platform&apos;s
              default of no inbound ports)
            </FieldLabel>
          </div>
        ) : null}
      </StepShell>
    )
  }

  if (step === 'confirm') {
    return (
      <StepShell
        title="Confirm"
        onBack={() => setStep('details')}
        onContinue={() => {
          if (!provider) return
          createProvision.mutate(
            {
              provider,
              region,
              size,
              name,
              role,
              control_plane_addr: controlPlaneAddr,
              ...(provider === 'aws'
                ? { allow_ssh_inbound: allowSSHInbound }
                : {}),
            },
            {
              onSuccess: (created) => {
                setProvisionId(created.id)
                setStep('progress')
              },
            },
          )
        }}
        continueLabel={createProvision.isPending ? 'Creating...' : 'Create'}
        continueDisabled={createProvision.isPending}
      >
        <dl className="grid grid-cols-2 gap-y-2 text-sm">
          <dt className="text-muted-foreground">Provider</dt>
          <dd>{provider ? PROVIDER_LABELS[provider] : '-'}</dd>
          <dt className="text-muted-foreground">Region</dt>
          <dd>{region}</dd>
          <dt className="text-muted-foreground">Size</dt>
          <dd>{size}</dd>
          <dt className="text-muted-foreground">Name</dt>
          <dd className="break-all font-mono">{name}</dd>
          <dt className="text-muted-foreground">Role</dt>
          <dd className="capitalize">{role}</dd>
          {provider === 'aws' ? (
            <>
              <dt className="text-muted-foreground">SSH inbound</dt>
              <dd>{allowSSHInbound ? 'Allowed' : 'Off (default)'}</dd>
            </>
          ) : null}
        </dl>
        {createProvision.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription>{createProvision.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </StepShell>
    )
  }

  // step === 'progress'
  const status = provision.data?.status
  return (
    <>
      <DialogHeader>
        <DialogTitle>Creating node</DialogTitle>
        <DialogDescription>
          This can take a few minutes: the server boots, installs Docker if
          needed, then enrolls with this control plane.
        </DialogDescription>
      </DialogHeader>
      <ProvisionProgress
        status={status}
        failureReason={provision.data?.failure_reason}
      />
      <DialogFooter>
        {status === 'ready' ? (
          <Button
            type="button"
            onClick={() => {
              onClose()
              if (provision.data?.node_id) {
                void navigate({
                  to: '/nodes/$id',
                  params: { id: provision.data.node_id },
                })
              }
            }}
          >
            View node
          </Button>
        ) : status === 'failed' ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => setStep('method')}
          >
            Start over
          </Button>
        ) : (
          <Button type="button" variant="outline" onClick={onClose}>
            Close (keeps provisioning)
          </Button>
        )}
      </DialogFooter>
    </>
  )
}
