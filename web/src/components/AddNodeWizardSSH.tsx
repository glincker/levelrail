import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  KeyGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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
import { SSHHostKeyVerify } from './AddNodeWizardSSHVerify'
import {
  useCreateSSHNodeProvision,
  usePreflightSSHNodeProvision,
  useSSHNodeProvision,
} from '../queries/nodeSSHProvision'
import type { SSHHostKey } from '../types/nodeSSHProvision'

type AuthMethod = 'key' | 'password'
// target -> verify (host key, read without credentials) -> form -> progress.
type Phase = 'target' | 'verify' | 'form' | 'progress'

// SSHEnrollFields is the wizard's third path: adopt a machine the
// operator already has by installing the agent over SSH. No credential is
// sent until the operator has confirmed the server's host key.
export function SSHEnrollFields({
  onDone,
  onBack,
}: {
  onDone: () => void
  onBack: () => void
}) {
  const { t } = useTranslation('nodes')
  const [phase, setPhase] = useState<Phase>('target')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('22')
  const [hostKeys, setHostKeys] = useState<SSHHostKey[]>([])
  const [fingerprint, setFingerprint] = useState('')
  const [confirmed, setConfirmed] = useState(false)
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

  const preflight = usePreflightSSHNodeProvision()
  const createProvision = useCreateSSHNodeProvision()
  const provision = useSSHNodeProvision(
    phase === 'progress' ? provisionId : null,
  )
  const portNumber = port ? Number(port) : 22

  const backToTarget = () => {
    setConfirmed(false)
    setFingerprint('')
    setHostKeys([])
    setPhase('target')
  }

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
            <Button type="button" variant="outline" onClick={backToTarget}>
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

  const header = (
    <DialogHeader>
      <DialogTitle className="flex items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={phase === 'target' ? onBack : backToTarget}
          aria-label="Back"
        >
          <ArrowLeftGlassIcon size="1.4em" />
        </Button>
        {phase === 'verify' ? t('sshVerify.title') : 'Connect over SSH'}
      </DialogTitle>
      <DialogDescription>
        Adopts a machine you already have by installing the node agent over SSH.
        The credential is used once for this install and never stored.
      </DialogDescription>
    </DialogHeader>
  )

  if (phase === 'target') {
    return (
      <>
        {header}
        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Field className="sm:col-span-2">
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
          {preflight.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>{preflight.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            disabled={host.trim() === '' || preflight.isPending}
            onClick={() => {
              preflight.mutate(
                { host: host.trim(), port: portNumber },
                {
                  onSuccess: (res) => {
                    setHostKeys(res.host_keys)
                    setFingerprint(res.host_keys[0]?.fingerprint ?? '')
                    setConfirmed(false)
                    setPhase('verify')
                  },
                },
              )
            }}
          >
            {preflight.isPending
              ? t('sshVerify.verifying')
              : t('sshVerify.verifyButton')}
          </Button>
        </DialogFooter>
      </>
    )
  }

  if (phase === 'verify') {
    return (
      <>
        {header}
        <div className="max-h-[60vh] overflow-y-auto pr-1">
          <SSHHostKeyVerify
            host={host.trim()}
            port={portNumber}
            keys={hostKeys}
            selected={fingerprint}
            onSelect={setFingerprint}
            confirmed={confirmed}
            onConfirmedChange={setConfirmed}
          />
        </div>
        <DialogFooter>
          <Button
            type="button"
            disabled={!confirmed || fingerprint === ''}
            onClick={() => setPhase('form')}
          >
            {t('sshVerify.continue')}
          </Button>
        </DialogFooter>
      </>
    )
  }

  const pinnedKey = hostKeys.find((k) => k.fingerprint === fingerprint)
  const nameValid = /^[a-z][a-z0-9-]*$/.test(name)
  const authValid =
    authMethod === 'key' ? privateKey.trim() !== '' : password !== ''
  const formValid =
    confirmed &&
    fingerprint !== '' &&
    username.trim() !== '' &&
    nameValid &&
    authValid &&
    controlPlaneAddr.trim() !== ''

  return (
    <>
      {header}
      <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
        <div className="flex items-center gap-2 rounded-lg border border-border p-2 text-xs">
          <ShieldCheckGlassIcon className="size-5 shrink-0 text-primary" />
          <span className="min-w-0 flex-1 font-mono break-all">
            {host.trim()}:{portNumber}{' '}
            {t('sshVerify.pinned', {
              type: pinnedKey?.type ?? '',
              fingerprint,
            })}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={backToTarget}
          >
            {t('sshVerify.change')}
          </Button>
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
            <KeyGlassIcon className="size-5" />
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
            <KeyGlassIcon className="size-5" />
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
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
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
                host: host.trim(),
                port: portNumber,
                username,
                auth,
                name,
                role,
                control_plane_addr: controlPlaneAddr,
                host_key_fingerprint: fingerprint,
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
