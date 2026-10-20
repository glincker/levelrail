import { useState, type FormEvent } from 'react'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Field, FieldLabel } from '@/components/ui/field'
import { TokenCreatedView } from '../TokenCreatedView'
import { InfoTip } from '../kit'
import { useCreateToken } from '../../queries/tokens'
import { AGENT_PRESETS, type AgentPreset } from '../../lib/agentConfig'
import type { CreateTokenResponse } from '../../types/token'

export function CreateAgentTokenDialog() {
  const [open, setOpen] = useState(false)
  const [agentName, setAgentName] = useState('')
  const [presetId, setPresetId] = useState<AgentPreset['id']>('observer')
  const [created, setCreated] = useState<CreateTokenResponse | null>(null)
  const createToken = useCreateToken()

  const preset =
    AGENT_PRESETS.find((p) => p.id === presetId) ?? AGENT_PRESETS[0]
  const trimmed = agentName.trim()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setCreated(null)
      setAgentName('')
      setPresetId('observer')
      createToken.reset()
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!trimmed || !preset) return
    createToken.mutate(
      {
        name: trimmed,
        abilities: preset.abilities,
        agent: { name: trimmed },
      },
      { onSuccess: setCreated },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Create agent token</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {created ? (
          <TokenCreatedView
            created={created}
            onDone={() => {
              handleOpenChange(false)
            }}
          />
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Create agent token</DialogTitle>
              <DialogDescription>
                A revocable credential for one AI agent, scoped by a preset.
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={submit} className="space-y-4">
              <Field>
                <div className="flex items-center gap-1">
                  <FieldLabel htmlFor="agent-token-name">Agent name</FieldLabel>
                  <InfoTip label="About agent names">
                    Shown here and on every audit log entry the token makes.
                  </InfoTip>
                </div>
                <Input
                  id="agent-token-name"
                  value={agentName}
                  maxLength={64}
                  placeholder="e.g. Claude Code"
                  onChange={(e) => {
                    setAgentName(e.target.value)
                  }}
                />
              </Field>

              <fieldset className="space-y-2">
                <legend className="text-sm font-medium">Scope</legend>
                {AGENT_PRESETS.map((p) => (
                  <label
                    key={p.id}
                    className="flex cursor-pointer items-start gap-2 rounded-lg border border-input p-2.5 has-checked:border-primary/40 has-checked:bg-primary/5"
                  >
                    <input
                      type="radio"
                      name="agent-preset"
                      className="mt-1"
                      checked={presetId === p.id}
                      onChange={() => {
                        setPresetId(p.id)
                      }}
                    />
                    <span className="space-y-0.5">
                      <span className="block text-sm font-medium">
                        {p.label}
                      </span>
                      <span className="block text-xs text-muted-foreground">
                        {p.hint} ({p.abilities.join(', ')})
                      </span>
                    </span>
                  </label>
                ))}
              </fieldset>

              {createToken.isError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>
                    {createToken.error.message}
                  </AlertDescription>
                </Alert>
              ) : null}

              <DialogFooter>
                <Button
                  type="submit"
                  disabled={createToken.isPending || !trimmed}
                >
                  {createToken.isPending ? 'Creating...' : 'Create token'}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
