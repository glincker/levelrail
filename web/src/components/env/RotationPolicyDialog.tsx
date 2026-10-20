import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type {
  RotationPolicy,
  RotationPolicyInput,
} from '../../queries/secretRotation'

const DEFAULT_INTERVAL_DAYS = 90
const MAX_INTERVAL_DAYS = 3650

export function RotationPolicyDialog({
  secretKey,
  policy,
  pending,
  error,
  onSave,
  onClose,
}: {
  secretKey: string
  policy?: RotationPolicy
  pending: boolean
  error: string | null
  onSave: (input: RotationPolicyInput) => void
  onClose: () => void
}) {
  const { t } = useTranslation('secretRotation')
  const [days, setDays] = useState(
    String(policy?.interval_days ?? DEFAULT_INTERVAL_DAYS),
  )
  const [grace, setGrace] = useState(String(policy?.grace_hours ?? 0))
  const [owner, setOwner] = useState(policy?.owner ?? '')
  const [auto, setAuto] = useState(policy?.auto_rotate ?? false)
  const [invalid, setInvalid] = useState(false)

  const submit = () => {
    const d = Number(days)
    const g = Number(grace)
    const valid =
      Number.isInteger(d) &&
      d >= 1 &&
      d <= MAX_INTERVAL_DAYS &&
      Number.isInteger(g) &&
      g >= 0
    setInvalid(!valid)
    if (!valid) return
    onSave({
      interval_days: d,
      grace_hours: g,
      owner: owner.trim(),
      auto_rotate: auto,
    })
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('dialog.title', { key: secretKey })}</DialogTitle>
          <DialogDescription>{t('dialog.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="rotation-days">{t('dialog.interval')}</Label>
            <Input
              id="rotation-days"
              type="number"
              inputMode="numeric"
              min={1}
              max={MAX_INTERVAL_DAYS}
              value={days}
              onChange={(e) => {
                setDays(e.target.value)
              }}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="rotation-grace">{t('dialog.grace')}</Label>
            <Input
              id="rotation-grace"
              type="number"
              inputMode="numeric"
              min={0}
              value={grace}
              onChange={(e) => {
                setGrace(e.target.value)
              }}
            />
            <p className="text-xs text-muted-foreground">
              {t('dialog.graceHint', { key: secretKey })}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="rotation-owner">{t('dialog.owner')}</Label>
            <Input
              id="rotation-owner"
              maxLength={120}
              placeholder={t('dialog.ownerPlaceholder')}
              value={owner}
              onChange={(e) => {
                setOwner(e.target.value)
              }}
            />
          </div>
          <div className="flex items-start justify-between gap-3">
            <div className="space-y-0.5">
              <Label htmlFor="rotation-auto">{t('dialog.auto')}</Label>
              <p className="text-xs text-muted-foreground">
                {t('dialog.autoHint')}
              </p>
            </div>
            <Switch
              id="rotation-auto"
              checked={auto}
              onCheckedChange={setAuto}
            />
          </div>
          {invalid ? (
            <p role="alert" className="text-sm text-destructive">
              {t('dialog.invalid')}
            </p>
          ) : null}
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('dialog.cancel')}
          </Button>
          <Button type="button" disabled={pending} onClick={submit}>
            {pending ? t('dialog.saving') : t('dialog.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
