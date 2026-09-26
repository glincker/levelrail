import { useState } from 'react'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { InfoTip } from '@/components/kit'
import { useCreateModelKey } from '../queries/modelKeys'
import {
  buildKeyRequest,
  EMPTY_KEY_FORM,
  EXPIRY_PRESETS,
  type KeyForm,
} from '../lib/modelKeyForm'

// Creates a named key. The plaintext comes back once and is handed to
// onCreated, which shows it in the reveal dialog.
export function CreateModelKeyDialog({
  modelName,
  onCreated,
}: {
  modelName: string
  onCreated: (apiKey: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<KeyForm>(EMPTY_KEY_FORM)
  const create = useCreateModelKey(modelName)

  function set(field: keyof KeyForm, value: string) {
    setForm((f) => ({ ...f, [field]: value }))
  }

  function submit() {
    create.mutate(buildKeyRequest(form, Date.now()), {
      onSuccess: (res) => {
        setOpen(false)
        setForm(EMPTY_KEY_FORM)
        onCreated(res.api_key)
      },
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) create.reset()
      }}
    >
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <PlusIcon aria-hidden="true" />
        New key
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New API key for &ldquo;{modelName}&rdquo;</DialogTitle>
          <DialogDescription>
            Leave a limit empty for no limit. The key is shown once.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <Field>
            <FieldLabel htmlFor="key-name">Name</FieldLabel>
            <Input
              id="key-name"
              value={form.name}
              placeholder="ci-runner"
              autoComplete="off"
              onChange={(e) => {
                set('name', e.target.value)
              }}
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="key-rpm">
                Requests/min
                <InfoTip label="About requests per minute">
                  Hard limit. A request over it gets 429 with a Retry-After
                  header. Empty means unlimited.
                </InfoTip>
              </FieldLabel>
              <Input
                id="key-rpm"
                inputMode="numeric"
                value={form.rpm}
                onChange={(e) => {
                  set('rpm', e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="key-tpm">
                Tokens/min
                <InfoTip label="About tokens per minute">
                  Soft limit, counted from finished responses. The request that
                  crosses it still completes; later ones get 429 until the
                  minute rolls over.
                </InfoTip>
              </FieldLabel>
              <Input
                id="key-tpm"
                inputMode="numeric"
                value={form.tpm}
                onChange={(e) => {
                  set('tpm', e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="key-tpd">
                Tokens/day
                <InfoTip label="About tokens per day">
                  Soft daily budget on a rolling 24 hour window, counted from
                  finished responses. Empty means unlimited.
                </InfoTip>
              </FieldLabel>
              <Input
                id="key-tpd"
                inputMode="numeric"
                value={form.tpd}
                onChange={(e) => {
                  set('tpd', e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="key-parallel">
                Parallel
                <InfoTip label="About parallel requests">
                  Most requests this key may run at the same time.
                </InfoTip>
              </FieldLabel>
              <Input
                id="key-parallel"
                inputMode="numeric"
                value={form.maxParallel}
                onChange={(e) => {
                  set('maxParallel', e.target.value)
                }}
              />
            </Field>
          </div>
          <FieldHint>
            Token limits are soft: they are enforced from metered responses, so
            a request can overshoot them.
          </FieldHint>
          <Field>
            <FieldLabel htmlFor="key-expires">Expires in (days)</FieldLabel>
            <div className="flex flex-wrap gap-1.5">
              {EXPIRY_PRESETS.map((p) => (
                <Button
                  key={p.label}
                  type="button"
                  size="xs"
                  variant={form.expiresDays === p.days ? 'default' : 'outline'}
                  onClick={() => {
                    set('expiresDays', p.days)
                  }}
                >
                  {p.label}
                </Button>
              ))}
            </div>
            <Input
              id="key-expires"
              inputMode="decimal"
              value={form.expiresDays}
              placeholder="never"
              onChange={(e) => {
                set('expiresDays', e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="key-paths">Allowed paths</FieldLabel>
            <Input
              id="key-paths"
              value={form.allowPaths}
              placeholder="/v1/chat/completions, /v1/embeddings"
              onChange={(e) => {
                set('allowPaths', e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="key-models">Allowed model names</FieldLabel>
            <Input
              id="key-models"
              value={form.allowModels}
              placeholder="any"
              onChange={(e) => {
                set('allowModels', e.target.value)
              }}
            />
          </Field>
          {create.error ? (
            <p role="alert" className="text-sm text-destructive">
              {create.error.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              type="submit"
              disabled={form.name.trim() === '' || create.isPending}
            >
              Create key
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
