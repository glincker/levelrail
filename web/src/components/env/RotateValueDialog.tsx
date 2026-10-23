import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export interface RotateValueDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  consequences: string[]
  valueLabel: string
  valueHint?: string
  /** True when the value is issued elsewhere and cannot be generated here. */
  valueRequired?: boolean
  pending: boolean
  error: string | null
  onSubmit: (value: string) => void
}

export function RotateValueDialog({
  open,
  onOpenChange,
  title,
  description,
  consequences,
  valueLabel,
  valueHint,
  valueRequired = false,
  pending,
  error,
  onSubmit,
}: RotateValueDialogProps) {
  const { t } = useTranslation('secretRotation')
  const [value, setValue] = useState('')
  const inputId = useId()

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setValue('')
        onOpenChange(next)
      }}
      title={title}
      description={description}
      consequences={consequences}
      confirmLabel={
        valueRequired || value
          ? t('rotateValue.confirm')
          : t('rotateValue.confirmGenerate')
      }
      pendingLabel={pending ? t('confirm.pending') : undefined}
      pending={pending || (valueRequired && value.trim() === '')}
      error={error}
      onConfirm={() => {
        onSubmit(value)
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor={inputId}>{valueLabel}</Label>
        <Input
          id={inputId}
          type="password"
          value={value}
          autoComplete="new-password"
          spellCheck={false}
          className="font-mono"
          placeholder={
            valueRequired ? undefined : t('rotateValue.generatePlaceholder')
          }
          onChange={(event) => {
            setValue(event.target.value)
          }}
        />
        {valueHint ? (
          <p className="text-xs text-muted-foreground">{valueHint}</p>
        ) : null}
      </div>
    </ConfirmDialog>
  )
}
