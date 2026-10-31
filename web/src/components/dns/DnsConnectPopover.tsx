import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useUpdateCloudflareDnsSettings } from '@/queries/cloudflareDns'

/** Pastes a scoped token once through the existing settings route; the token is cleared on submit and never shown back. */
export function DnsConnectPopover({
  provider,
  label,
  children,
  variant = 'default',
  size = 'sm',
}: {
  provider: string
  label: string
  children: ReactNode
  variant?: 'default' | 'outline' | 'ghost'
  size?: 'sm' | 'xs'
}) {
  const { t } = useTranslation('dnsSetup')
  const qc = useQueryClient()
  const save = useUpdateCloudflareDnsSettings()
  const [open, setOpen] = useState(false)
  const [token, setToken] = useState('')
  const [failed, setFailed] = useState(false)
  const supported = provider === 'cloudflare'

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const value = token.trim()
    if (!value) return
    setToken('')
    setFailed(false)
    save.mutate(
      { enabled: true, token: value },
      {
        onSuccess: () => {
          setOpen(false)
          void qc.invalidateQueries({ queryKey: ['dns-setup'] })
        },
        onError: () => setFailed(true),
      },
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={<Button size={size} variant={variant} />}>
        {children}
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 space-y-2">
        {supported ? (
          <form onSubmit={submit} className="space-y-2">
            <p className="text-xs text-muted-foreground">
              {t('connect.permissions', { provider: label })}
            </p>
            <Label htmlFor="dns-token" className="text-xs">
              {t('connect.tokenLabel', { provider: label })}
            </Label>
            <Input
              id="dns-token"
              type="password"
              autoComplete="off"
              spellCheck={false}
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
            {failed && (
              <p role="alert" className="text-xs text-tone-danger">
                {t('connect.failed')}
              </p>
            )}
            <Button
              type="submit"
              size="sm"
              disabled={!token.trim() || save.isPending}
            >
              {save.isPending ? t('connect.saving') : t('connect.save')}
            </Button>
          </form>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('connect.useSettings', { provider: label })}
          </p>
        )}
      </PopoverContent>
    </Popover>
  )
}
