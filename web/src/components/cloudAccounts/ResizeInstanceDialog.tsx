import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import type { CloudInstance, CloudResizeInput } from '../../types/cloudAccounts'

// Example size, not translatable text.
const EX_TYPE = 't3.medium'

export function ResizeInstanceDialog({
  instance,
  provider,
  byType,
  pending,
  error,
  onOpenChange,
  onSubmit,
}: Readonly<{
  instance: CloudInstance | null
  provider: string
  /** The driver resizes by instance type; otherwise by ocpus and memory. */
  byType: boolean
  pending: boolean
  error: string | null
  onOpenChange: (open: boolean) => void
  onSubmit: (spec: CloudResizeInput) => void
}>) {
  const { t } = useTranslation('cloudAccounts')
  const uid = useId()
  const [type, setType] = useState('')
  const [ocpus, setOcpus] = useState('')
  const [mem, setMem] = useState('')
  if (!instance) return null
  const spec: CloudResizeInput = byType
    ? { instance_type: type.trim() }
    : { ocpus: Number(ocpus), memory_gb: Number(mem) }
  const valid = byType
    ? type.trim() !== ''
    : Number(ocpus) > 0 && Number(mem) > 0
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent size="confirm">
        <DialogHeader>
          <DialogTitle>
            {t('instances.resize.title', {
              name: instance.name || instance.id,
            })}
          </DialogTitle>
          <DialogDescription>
            {t('instances.resize.current', { shape: instance.shape })}
          </DialogDescription>
        </DialogHeader>
        {provider === 'oracle' ? (
          <Alert variant="warning">
            <AlertDescription>
              {t('instances.resize.oracleReboot')}
            </AlertDescription>
          </Alert>
        ) : null}
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (valid) onSubmit(spec)
          }}
        >
          {byType ? (
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor={`${uid}-type`}>
                {t('instances.resize.type')}
              </Label>
              <Input
                id={`${uid}-type`}
                value={type}
                placeholder={EX_TYPE}
                className="font-mono"
                onChange={(e) => {
                  setType(e.target.value)
                }}
              />
            </div>
          ) : (
            <>
              <div className="space-y-1">
                <Label htmlFor={`${uid}-ocpus`}>
                  {t('instances.resize.ocpus')}
                </Label>
                <Input
                  id={`${uid}-ocpus`}
                  inputMode="decimal"
                  value={ocpus}
                  onChange={(e) => {
                    setOcpus(e.target.value)
                  }}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor={`${uid}-mem`}>
                  {t('instances.resize.memory')}
                </Label>
                <Input
                  id={`${uid}-mem`}
                  inputMode="decimal"
                  value={mem}
                  onChange={(e) => {
                    setMem(e.target.value)
                  }}
                />
              </div>
            </>
          )}
          {error ? (
            <p role="alert" className="text-sm text-destructive sm:col-span-2">
              {error}
            </p>
          ) : null}
          <DialogFooter className="sm:col-span-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                onOpenChange(false)
              }}
            >
              {t('instances.cancel')}
            </Button>
            <Button type="submit" disabled={!valid || pending}>
              {t('instances.resize.confirm')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
