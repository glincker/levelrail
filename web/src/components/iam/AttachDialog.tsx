import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  attachPrincipal,
  detachPrincipal,
  iamPolicyKeys,
} from '../../queries/iamPolicies'
import type { PolicyResource, PrincipalType } from '../../queries/iamPolicies'
import { iamBuilderKeys } from '../../queries/iamBuilder'
import type { IamPrincipal, PreviewResult } from '../../queries/iamBuilder'
import { ImpactPreview } from './ImpactPreview'
import { PrincipalChecklist } from './PrincipalChecklist'

type Mode = 'attach' | 'detach'

function splitKey(k: string): { type: PrincipalType; id: string } {
  const [type, ...rest] = k.split(':')
  return { type: type === 'user' ? 'user' : 'token', id: rest.join(':') }
}

/** AttachDialog attaches or detaches one policy for many principals at once, showing the access each one gains or loses before it confirms. */
export function AttachDialog({
  open,
  onOpenChange,
  mode,
  policies,
  principals,
  policyId,
  preselected,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: Mode
  policies: PolicyResource[]
  principals: IamPrincipal[]
  policyId?: string
  preselected?: string[]
}) {
  const { t } = useTranslation('iam')
  const queryClient = useQueryClient()
  const [pid, setPid] = useState(policyId ?? '')
  const [selected, setSelected] = useState<string[]>(preselected ?? [])
  const [preview, setPreview] = useState<PreviewResult | undefined>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const policy = policies.find((p) => p.id === pid)
  const candidates = useMemo(
    () =>
      principals.filter((p) => {
        const has = p.policy_ids.includes(pid)
        return mode === 'attach' ? !has : has
      }),
    [principals, pid, mode],
  )
  const refs = selected.map((k) => {
    const s = splitKey(k)
    return { principal_type: s.type, principal_id: s.id }
  })
  const request =
    mode === 'attach'
      ? { policy_id: pid, attach: refs }
      : { policy_id: pid, detach: refs }
  const blocked = preview?.guard.blocked ?? false

  const confirm = async () => {
    if (!policy || refs.length === 0) return
    setBusy(true)
    setError(null)
    let done = 0
    try {
      for (const r of refs) {
        const body = {
          policyId: policy.id,
          principal_type: r.principal_type,
          principal_id: r.principal_id,
        }
        if (mode === 'attach') await attachPrincipal(body)
        else await detachPrincipal(body)
        done += 1
      }
      toast.add({
        title: t(
          mode === 'attach' ? 'attachDialog.attached' : 'attachDialog.detached',
          { name: policy.name, count: done },
        ),
        type: 'success',
      })
      onOpenChange(false)
    } catch (e) {
      setError(
        t('attachDialog.failed', {
          message: e instanceof Error ? e.message : t('errors.generic'),
        }),
      )
    } finally {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: iamPolicyKeys.all }),
        queryClient.invalidateQueries({ queryKey: iamBuilderKeys.all }),
      ])
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t(
              mode === 'attach'
                ? 'attachDialog.attachTitle'
                : 'attachDialog.detachTitle',
            )}
          </DialogTitle>
          <DialogDescription>{t('attachDialog.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="attach-policy">
              {t('attachDialog.policy')}
            </FieldLabel>
            <Select
              value={pid}
              onValueChange={(v) => {
                setPid(String(v))
                setSelected([])
              }}
              disabled={Boolean(policyId)}
            >
              <SelectTrigger id="attach-policy" className="w-full">
                <SelectValue placeholder={t('attachDialog.policyPick')} />
              </SelectTrigger>
              <SelectContent>
                {policies.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          {pid ? (
            <PrincipalChecklist
              principals={candidates}
              selected={selected}
              onChange={setSelected}
              title={t('attachDialog.principals')}
              description={
                selected.length === 0 ? t('attachDialog.noPrincipals') : ''
              }
            />
          ) : null}
          <ImpactPreview
            request={request}
            enabled={open && Boolean(pid) && refs.length > 0}
            onResult={setPreview}
          />
          {error ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('builder.cancel')}
          </Button>
          <Button
            type="button"
            variant={mode === 'detach' ? 'destructive' : 'default'}
            onClick={() => void confirm()}
            disabled={busy || refs.length === 0 || blocked || !policy}
          >
            {busy
              ? t('attachDialog.working')
              : t(
                  mode === 'attach'
                    ? 'attachDialog.confirmAttach'
                    : 'attachDialog.confirmDetach',
                )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
