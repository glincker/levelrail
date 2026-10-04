import { useState } from 'react'
import {
  ArrowLeftIcon,
  KeyIcon,
  PasswordIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SSHProvisionProgress } from './AddNodeWizardSteps'
import {
  useCreateSSHNodeProvision,
  useSSHNodeProvision,
} from '../queries/nodeSSHProvision'

type AuthMethod = 'key' | 'password'
type Phase = 'form' | 'progress'

// SSHEnrollFields is the wizard's third path: adopt a machine the
// operator already has (any VPS, home server, Raspberry Pi) instead of
// creating one at a cloud provider (the 'method' step's own two other
// buttons) or copy-pasting a join token by hand onto it themselves
// (ManualEnrollFields). It owns a small phase machine of its own
// (form -> progress) rather than adding more values to AddNodeWizard's
// own Step type, the same self-contained shape ManualEnrollFields
// already uses for the manual path.
export function SSHEnrollFields({
  onDone,
  onBack,
}: {
  onDone: () => void
  onBack: () => void
}) {
  const [phase, setPhase] = useState<Phase>('form')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('22')
  const [username, setUsername] = useState('root')
  const [authMethod, setAuthMethod] = useState<AuthMethod>('key')
  const [privateKey, setPrivateKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [password, setPassword] = useState('')
  const [name, setName] = useState('')
  const [role, setRole] = useState<'general' | 'build'>('general')
  const [controlPlaneAddr, setControlPlaneAddr] = useState(
    () => `${window.location.hostname}:9443`,
  )
  const [provisionId, setProvisionId] = useState<string | null>(null)

  const createProvision = useCreateSSHNodeProvision()
  const provision = useSSHNodeProvision(
    phase === 'progress' ? provisionId : null,
  )

  if (phase === 'progress') {
    const status = provision.data?.status
    return (
      <>
        <DialogHeader>
          <DialogTitle>Connecting over SSH</DialogTitle>
          <DialogDescription>
            This can take a few minutes: connecting, detecting the remote host,
            installing Docker if needed, then enrolling with this control plane.
          </DialogDescription>
        </DialogHeader>
        <SSHProvisionProgress
          status={status}
          detectedOS={provision.data?.detected_os}
          detectedArch={provision.data?.detected_arch}
          log={provision.data?.log}
          failureReason={provision.data?.failure_reason}
        />
        <DialogFooter>
          {status === 'ready' ? (
            <Button type="button" onClick={onDone}>
              Done
            </Button>
          ) : status === 'failed' ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => setPhase('form')}
            >
              Try again
            </Button>
          ) : (
            <Button type="button" variant="outline" onClick={onDone}>
              Close (keeps provisioning)
            </Button>
          )}
        </DialogFooter>
      </>
    )
  }

  const nameValid = /^[a-z][a-z0-9-]*$/.test(name)
  const authValid =
    authMethod === 'key' ? privateKey.trim() !== '' : password !== ''
  const formValid =
    host.trim() !== '' &&
    username.trim() !== '' &&
    nameValid &&
    authValid &&
    controlPlaneAddr.trim() !== ''

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onBack}
            aria-label="Back"
          >
            <ArrowLeftIcon />
          </Button>
          Connect over SSH
        </DialogTitle>
        <DialogDescription>
          Adopts a machine you already have by installing the node agent over
          SSH. The credential is used once for this install and never stored.
        </DialogDescription>
      </DialogHeader>
      <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
        <div className="grid grid-cols-3 gap-3">
          <Field className="col-span-2">
            <FieldLabel htmlFor="ssh-host">Host</FieldLabel>
            <Input
              id="ssh-host"
              value={host}
              onChange={(e) => setHost(e.target.value)}
              placeholder="192.0.2.10"
              className="font-mono text-xs"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="ssh-port">Port</FieldLabel>
            <Input
              id="ssh-port"
              value={port}
              onChange={(e) => setPort(e.target.value.replace(/[^0-9]/g, ''))}
              className="font-mono text-xs"
            />
          </Field>
        </div>
        <Field>
          <FieldLabel htmlFor="ssh-username">Username</FieldLabel>
          <Input
            id="ssh-username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="root"
            className="font-mono text-xs"
          />
        </Field>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setAuthMethod('key')}
            className={`flex flex-1 items-center justify-center gap-2 rounded-lg border p-2 text-sm transition-colors ${
              authMethod === 'key'
                ? 'border-primary bg-primary/5'
                : 'border-border hover:bg-muted/50'
            }`}
          >
            <KeyIcon className="size-4" />
            Private key
          </button>
          <button
            type="button"
            onClick={() => setAuthMethod('password')}
            className={`flex flex-1 items-center justify-center gap-2 rounded-lg border p-2 text-sm transition-colors ${
              authMethod === 'password'
                ? 'border-primary bg-primary/5'
                : 'border-border hover:bg-muted/50'
            }`}
          >
            <PasswordIcon className="size-4" />
            Password
          </button>
        </div>
        {authMethod === 'key' ? (
          <>
            <Field>
              <FieldLabel htmlFor="ssh-key">Private key</FieldLabel>
              <Textarea
                id="ssh-key"
                value={privateKey}
                onChange={(e) => setPrivateKey(e.target.value)}
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                className="min-h-28 font-mono text-xs"
              />
              <FieldDescription>
                Sent once over this same authenticated connection and never
                stored.
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="ssh-passphrase">
                Passphrase (optional)
              </FieldLabel>
              <Input
                id="ssh-passphrase"
                type="password"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
              />
            </Field>
          </>
        ) : (
          <Field>
            <FieldLabel htmlFor="ssh-password">Password</FieldLabel>
            <Input
              id="ssh-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <FieldDescription>
              Sent once over this same authenticated connection and never
              stored.
            </FieldDescription>
          </Field>
        )}
        <Field>
          <FieldLabel htmlFor="ssh-name">Name</FieldLabel>
          <Input
            id="ssh-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="home-server"
            className="font-mono text-xs"
          />
          <FieldDescription>
            Lowercase letters, digits and hyphens only.
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="ssh-role">Role</FieldLabel>
          <Select
            value={role}
            onValueChange={(v) =>
              setRole((v as 'general' | 'build') ?? 'general')
            }
          >
            <SelectTrigger id="ssh-role" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="general">General</SelectItem>
              <SelectItem value="build">Build</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="ssh-cp-addr">Control plane address</FieldLabel>
          <Input
            id="ssh-cp-addr"
            value={controlPlaneAddr}
            onChange={(e) => setControlPlaneAddr(e.target.value)}
            className="font-mono text-xs"
          />
          <FieldDescription>
            Host and port the adopted machine dials to reach this control plane,
            port 9443 by default.
          </FieldDescription>
        </Field>
        {createProvision.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{createProvision.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </div>
      <DialogFooter>
        <Button
          type="button"
          disabled={!formValid || createProvision.isPending}
          onClick={() => {
            const auth =
              authMethod === 'key'
                ? {
                    type: 'key' as const,
                    private_key: privateKey,
                    ...(passphrase ? { passphrase } : {}),
                  }
                : { type: 'password' as const, password }
            createProvision.mutate(
              {
                host,
                port: port ? Number(port) : undefined,
                username,
                auth,
                name,
                role,
                control_plane_addr: controlPlaneAddr,
              },
              {
                onSuccess: (created) => {
                  setProvisionId(created.id)
                  setPhase('progress')
                },
              },
            )
          }}
        >
          {createProvision.isPending ? 'Connecting...' : 'Connect'}
        </Button>
      </DialogFooter>
    </>
  )
}
