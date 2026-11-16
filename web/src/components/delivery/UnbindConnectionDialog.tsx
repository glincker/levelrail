import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import { useAppBindings } from '../../queries/appDelivery'
import { useAppListOptional } from '../../queries/apps'
import { useUnbindConnection } from '../../queries/bindings'

interface UnbindProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Connection ref; the picker lists the app's own bindings of it. */
  connectionRef: string
  label: string
  /** Fixed app and binding id (Delivery strip); without them the dialog asks. */
  app?: string
  bindingId?: string
}

// DELETE /api/v1/apps/{name}/bindings/{id}. Org defaults are not removed here.
export function UnbindConnectionDialog({
  open,
  onOpenChange,
  connectionRef,
  label,
  app,
  bindingId,
}: Readonly<UnbindProps>) {
  const { t } = useTranslation('delivery')
  const ids = { app: useId(), binding: useId() }
  const apps = useAppListOptional()
  const unbind = useUnbindConnection()
  const [pickedApp, setPickedApp] = useState('')
  const [pickedId, setPickedId] = useState('')
  const target = app ?? pickedApp
  const own = useAppBindings(app === undefined ? pickedApp : '')
  const rows = (own.data?.bindings ?? []).filter(
    (b) => b.ref === connectionRef && b.id !== undefined,
  )
  const id = bindingId ?? pickedId

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) unbind.reset()
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('unbind.title', { name: label })}
      description={t('unbind.description')}
      tone="destructive"
      focusCancel
      confirmLabel={t('unbind.confirm')}
      pendingLabel={t('unbind.unbinding')}
      pending={unbind.isPending}
      confirmDisabled={target === '' || id === ''}
      error={unbind.isError ? unbind.error.message : null}
      onConfirm={() => {
        unbind.mutate(
          { app: target, id },
          {
            onSuccess: () => {
              onOpenChange(false)
              toast.add({
                title: t('unbind.toast', { name: label, app: target }),
                type: 'success',
              })
            },
          },
        )
      }}
    >
      {app === undefined ? (
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor={ids.app}>{t('bind.app')}</Label>
            <NativeSelect
              id={ids.app}
              className="w-full"
              value={pickedApp}
              onChange={(e) => {
                setPickedApp(e.target.value)
                setPickedId('')
              }}
            >
              <option value="">{t('bind.appPlaceholder')}</option>
              {(apps.data ?? []).map((a) => (
                <option key={a.name} value={a.name}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </div>
          {pickedApp !== '' ? (
            <div className="space-y-1.5">
              <Label htmlFor={ids.binding}>{t('unbind.binding')}</Label>
              <NativeSelect
                id={ids.binding}
                className="w-full"
                value={pickedId}
                onChange={(e) => setPickedId(e.target.value)}
              >
                <option value="">
                  {rows.length === 0
                    ? t('unbind.noneForApp')
                    : t('unbind.pick')}
                </option>
                {rows.map((b) => (
                  <option key={b.id} value={b.id}>
                    {t(`bind.roles.${b.role}`, { defaultValue: b.role })}
                  </option>
                ))}
              </NativeSelect>
            </div>
          ) : null}
        </div>
      ) : null}
    </ConfirmDialog>
  )
}
