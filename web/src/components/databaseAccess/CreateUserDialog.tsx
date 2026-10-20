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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import type {
  AccessPreset,
  DatabaseCredential,
} from '../../types/databaseAccess'
import { useCreateDatabaseUser } from '../../queries/databaseAccess'

const NAME_PATTERN = /^[a-z][a-z0-9_]{2,39}$/
const PRESETS: AccessPreset[] = ['read_only', 'read_write', 'owner']
const EXPIRY_DAYS = ['0', '7', '30', '90'] as const
const DEFAULT_LIMIT = 20
const MS_PER_DAY = 86_400_000

type ExpiryDays = (typeof EXPIRY_DAYS)[number]

const EXPIRY_KEYS: Record<
  ExpiryDays,
  'expiryNever' | 'expiry7' | 'expiry30' | 'expiry90'
> = {
  '0': 'expiryNever',
  '7': 'expiry7',
  '30': 'expiry30',
  '90': 'expiry90',
}

/** CreateUserDialog creates a login with a privilege preset; the generated password is handed to onCreated once. */
export function CreateUserDialog({
  databaseName,
  open,
  onOpenChange,
  onCreated,
}: {
  databaseName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (credential: DatabaseCredential) => void
}) {
  const { t } = useTranslation('databaseAccess')
  const create = useCreateDatabaseUser(databaseName)
  const [name, setName] = useState('')
  const [preset, setPreset] = useState<AccessPreset>('read_only')
  const [limit, setLimit] = useState(String(DEFAULT_LIMIT))
  const [expiry, setExpiry] = useState<ExpiryDays>('0')

  const nameValid = NAME_PATTERN.test(name)
  const limitNum = Number(limit)
  const limitValid = Number.isInteger(limitNum) && limitNum >= 0

  function submit() {
    const days = Number(expiry)
    create.mutate(
      {
        name,
        preset,
        connection_limit: limitNum || undefined,
        expires_at:
          days > 0
            ? new Date(Date.now() + days * MS_PER_DAY).toISOString()
            : undefined,
      },
      {
        onSuccess: (res) => {
          setName('')
          onOpenChange(false)
          onCreated(res.credential)
        },
        onError: (err) =>
          toast.add({
            title: t('create.failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="db-user-name">{t('create.name')}</Label>
            <Input
              id="db-user-name"
              value={name}
              autoComplete="off"
              spellCheck={false}
              className="font-mono"
              aria-invalid={name !== '' && !nameValid}
              onChange={(e) => setName(e.target.value.toLowerCase())}
            />
            <p className="text-xs text-muted-foreground">
              {t('create.nameHint')}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="db-user-preset">{t('create.preset')}</Label>
            <Select
              value={preset}
              onValueChange={(v) => setPreset(v as AccessPreset)}
            >
              <SelectTrigger id="db-user-preset" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PRESETS.map((p) => (
                  <SelectItem key={p} value={p}>
                    {t(`users.preset.${p}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t(`users.presetHint.${preset}`)}
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="db-user-limit">{t('create.limit')}</Label>
              <Input
                id="db-user-limit"
                inputMode="numeric"
                value={limit}
                onChange={(e) => setLimit(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                {t('create.limitHint')}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="db-user-expiry">{t('create.expiry')}</Label>
              <Select
                value={expiry}
                onValueChange={(v) => setExpiry(v as ExpiryDays)}
              >
                <SelectTrigger id="db-user-expiry" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {EXPIRY_DAYS.map((d) => (
                    <SelectItem key={d} value={d}>
                      {t(`create.${EXPIRY_KEYS[d]}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('users.cancel')}
          </Button>
          <Button
            disabled={!nameValid || !limitValid || create.isPending}
            onClick={submit}
          >
            {create.isPending ? t('create.submitting') : t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
