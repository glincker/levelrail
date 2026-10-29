import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  EyeGlassIcon,
  EyeOffGlassIcon,
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
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { DraftBanner } from '../DraftBanner'
import { useDraft } from '../../hooks/useDraft'
import {
  useCreateNetworkShare,
  useNetworkSharesOptional,
} from '../../queries/networkShares'
import type { NetworkShareProtocol } from '../../types/networkShare'
import { FieldError, FieldHelp } from './Fields'
import {
  isReadOnly,
  normalizePath,
  remoteAddress,
  suggestName,
  validate,
  type ShareValues,
} from './logic'
import { MountOptions } from './MountOptions'
import { ProtocolCards } from './ProtocolCards'

const INITIAL: ShareValues = {
  protocol: 'nfs',
  name: '',
  host: '',
  path: '',
  username: '',
  password: '',
  options: '',
}

export function ShareFlow({ onClose }: Readonly<{ onClose: () => void }>) {
  const { t } = useTranslation('settings')
  const draft = useDraft('network-share', INITIAL)
  const values = draft.value
  const setValues = draft.setValue
  const [nameTouched, setNameTouched] = useState(false)
  const [touched, setTouched] = useState<Set<keyof ShareValues>>(new Set())
  const [attempted, setAttempted] = useState(false)
  const [reveal, setReveal] = useState(false)
  const create = useCreateNetworkShare()
  const existing = useNetworkSharesOptional()
  const takenNames = (existing.data ?? []).map((s) => s.name)
  const errors = validate(values, takenNames)
  const isSmb = values.protocol === 'cifs'

  function update(patch: Partial<ShareValues>) {
    setValues((prev) => {
      const next = { ...prev, ...patch }
      if (
        !nameTouched &&
        (patch.host !== undefined || patch.path !== undefined)
      ) {
        next.name = suggestName(next.host, next.path)
      }
      return next
    })
  }

  function blur(field: keyof ShareValues) {
    setTouched((prev) => new Set(prev).add(field))
  }

  function chooseProtocol(protocol: NetworkShareProtocol) {
    if (protocol === values.protocol) return
    setTouched(new Set())
    setAttempted(false)
    setValues((prev) => ({
      ...prev,
      protocol,
      path: '',
      options: '',
      username: '',
      password: '',
      name: nameTouched ? prev.name : suggestName(prev.host, ''),
    }))
  }

  function shown(field: keyof ShareValues) {
    return attempted || touched.has(field) ? errors[field] : undefined
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (Object.keys(errors).length > 0) {
      setAttempted(true)
      return
    }
    const name = values.name.trim()
    create.mutate(
      {
        name,
        protocol: values.protocol,
        host: values.host.trim(),
        remote_path: normalizePath(values.protocol, values.path),
        mount_options: values.options.trim() || undefined,
        username: isSmb ? values.username.trim() : undefined,
        password: isSmb ? values.password : undefined,
      },
      {
        onSuccess: () => {
          draft.clearOnSuccess()
          onClose()
          toast.add({
            title: t('networkSharesPage.dialog.added', { name }),
            type: 'success',
          })
        },
      },
    )
  }

  const nameFree =
    existing.isSuccess && values.name.trim() !== '' && !errors.name
  const ready =
    values.host.trim() !== '' &&
    values.path.trim() !== '' &&
    values.name.trim() !== ''
  const pathKind = isSmb ? 'smb' : 'nfs'

  return (
    <form
      noValidate
      onSubmit={onSubmit}
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-4"
    >
      <DialogHeader>
        <DialogTitle>{t('networkSharesPage.dialog.title')}</DialogTitle>
        <DialogDescription>
          {t('networkSharesPage.dialog.description')}
        </DialogDescription>
      </DialogHeader>
      <div className="-mx-1 min-h-0 min-w-0 flex-1 space-y-4 overflow-y-auto px-1">
        <DraftBanner draft={draft} onRestore={() => setNameTouched(true)} />
        <ProtocolCards value={values.protocol} onChange={chooseProtocol} />

        <div className="grid min-w-0 gap-4 sm:grid-cols-2">
          <div className="min-w-0 space-y-1.5">
            <Label htmlFor="share-host">
              {t('networkSharesPage.dialog.host')}
            </Label>
            <Input
              id="share-host"
              className="font-mono"
              value={values.host}
              autoComplete="off"
              autoCapitalize="off"
              spellCheck={false}
              placeholder={t('networkSharesPage.dialog.hostPlaceholder')}
              aria-invalid={shown('host') ? true : undefined}
              aria-describedby="share-host-help share-host-error"
              onChange={(e) => {
                update({ host: e.target.value })
              }}
              onBlur={() => {
                blur('host')
              }}
            />
            <FieldError id="share-host-error" messageKey={shown('host')} />
            <FieldHelp id="share-host-help">
              {t('networkSharesPage.dialog.hostHint')}
            </FieldHelp>
          </div>
          <div className="min-w-0 space-y-1.5">
            <Label htmlFor="share-path">
              {t(`networkSharesPage.dialog.path.${pathKind}`)}
            </Label>
            <Input
              id="share-path"
              className="font-mono"
              value={values.path}
              autoComplete="off"
              autoCapitalize="off"
              spellCheck={false}
              placeholder={t(
                `networkSharesPage.dialog.pathPlaceholder.${pathKind}`,
              )}
              aria-invalid={shown('path') ? true : undefined}
              aria-describedby="share-path-help share-path-error"
              onChange={(e) => {
                update({ path: e.target.value })
              }}
              onBlur={() => {
                blur('path')
              }}
            />
            <FieldError id="share-path-error" messageKey={shown('path')} />
            <FieldHelp id="share-path-help">
              {t(`networkSharesPage.dialog.pathHint.${pathKind}`)}
            </FieldHelp>
          </div>
        </div>

        {isSmb ? (
          <div className="grid min-w-0 gap-4 sm:grid-cols-2">
            <div className="min-w-0 space-y-1.5">
              <Label htmlFor="share-username">
                {t('networkSharesPage.dialog.username')}
              </Label>
              <Input
                id="share-username"
                value={values.username}
                autoComplete="off"
                autoCapitalize="off"
                spellCheck={false}
                aria-invalid={shown('username') ? true : undefined}
                aria-describedby="share-username-error"
                onChange={(e) => {
                  update({ username: e.target.value })
                }}
                onBlur={() => {
                  blur('username')
                }}
              />
              <FieldError
                id="share-username-error"
                messageKey={shown('username')}
              />
            </div>
            <div className="min-w-0 space-y-1.5">
              <Label htmlFor="share-password">
                {t('networkSharesPage.dialog.password')}
              </Label>
              <div className="relative">
                <Input
                  id="share-password"
                  type={reveal ? 'text' : 'password'}
                  className="pr-9 font-mono"
                  value={values.password}
                  autoComplete="off"
                  aria-invalid={shown('password') ? true : undefined}
                  aria-describedby="share-password-help share-password-error"
                  onChange={(e) => {
                    update({ password: e.target.value })
                  }}
                  onBlur={() => {
                    blur('password')
                  }}
                />
                <button
                  type="button"
                  aria-pressed={reveal}
                  aria-label={t(
                    reveal
                      ? 'networkSharesPage.dialog.hidePassword'
                      : 'networkSharesPage.dialog.showPassword',
                  )}
                  onClick={() => {
                    setReveal((v) => !v)
                  }}
                  className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground hover:text-foreground"
                >
                  {reveal ? (
                    <EyeOffGlassIcon className="size-4.5" aria-hidden="true" />
                  ) : (
                    <EyeGlassIcon className="size-4.5" aria-hidden="true" />
                  )}
                </button>
              </div>
              <FieldError
                id="share-password-error"
                messageKey={shown('password')}
              />
              <FieldHelp id="share-password-help">
                {t('networkSharesPage.dialog.passwordHint')}
              </FieldHelp>
            </div>
          </div>
        ) : null}

        <div className="min-w-0 space-y-1.5">
          <Label htmlFor="share-name">
            {t('networkSharesPage.dialog.name')}
          </Label>
          <Input
            id="share-name"
            className="font-mono"
            value={values.name}
            autoComplete="off"
            autoCapitalize="off"
            spellCheck={false}
            placeholder={t('networkSharesPage.dialog.namePlaceholder')}
            aria-invalid={shown('name') ? true : undefined}
            aria-describedby="share-name-help share-name-error"
            onChange={(e) => {
              setNameTouched(true)
              update({ name: e.target.value })
            }}
            onBlur={() => {
              blur('name')
            }}
          />
          <FieldError id="share-name-error" messageKey={shown('name')} />
          {nameFree ? (
            <p className="flex items-center gap-1 text-xs text-muted-foreground">
              <CircleCheckGlassIcon
                aria-hidden="true"
                className="size-4 text-tone-success"
              />
              {t('networkSharesPage.dialog.nameFree')}
            </p>
          ) : (
            <FieldHelp id="share-name-help">
              {t('networkSharesPage.dialog.nameHint')}
            </FieldHelp>
          )}
        </div>

        <MountOptions
          protocol={values.protocol}
          value={values.options}
          onChange={(options) => {
            update({ options })
          }}
        />

        {create.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
              aria-hidden="true"
            />
            <AlertDescription>{create.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </div>
      <p
        data-testid="share-summary"
        className="min-w-0 break-words rounded-md bg-muted px-3 py-2 font-mono text-xs text-muted-foreground"
      >
        {ready
          ? t('networkSharesPage.dialog.summary', {
              remote: remoteAddress(values.protocol, values.host, values.path),
              mode: t(
                isReadOnly(values.options)
                  ? 'networkSharesPage.dialog.readOnly'
                  : 'networkSharesPage.dialog.readWrite',
              ),
              name: values.name.trim(),
            })
          : t('networkSharesPage.dialog.summaryEmpty')}
      </p>
      <DialogFooter className="sm:flex-wrap">
        <Button type="button" variant="outline" onClick={onClose}>
          {t('networkSharesPage.dialog.cancel')}
        </Button>
        <Button type="submit" disabled={create.isPending}>
          {create.isPending
            ? t('networkSharesPage.dialog.submitting')
            : t('networkSharesPage.dialog.submit')}
        </Button>
      </DialogFooter>
    </form>
  )
}
