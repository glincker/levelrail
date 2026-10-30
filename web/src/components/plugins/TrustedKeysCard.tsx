import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import {
  useAddPluginKey,
  usePluginKeys,
  useRemovePluginKey,
  type PluginKey,
} from '../../queries/plugins'

export function TrustedKeysCard() {
  const { t } = useTranslation('plugins')
  const { data: keys = [] } = usePluginKeys()
  const add = useAddPluginKey()
  const remove = useRemovePluginKey()
  const [name, setName] = useState('')
  const [publicKey, setPublicKey] = useState('')
  const [target, setTarget] = useState<PluginKey | null>(null)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('keys.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('keys.description')}</p>
      </CardHeader>
      <CardContent className="space-y-4">
        {keys.length === 0 ? (
          <EmptyState
            className="gap-2 border-0 bg-transparent py-4"
            icon={<KeyGlassIcon className="size-6" />}
            title={t('keys.emptyTitle')}
            description={t('keys.emptyDescription')}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('keys.colName')}</TableHead>
                <TableHead>{t('keys.colKeyId')}</TableHead>
                <TableHead>{t('keys.colAdded')}</TableHead>
                <TableHead className="text-right">
                  <span className="sr-only">{t('keys.remove')}</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.map((k) => (
                <TableRow key={k.key_id}>
                  <TableCell className="font-medium">{k.name}</TableCell>
                  <TableCell>
                    <code className="text-xs break-all">{k.key_id}</code>
                  </TableCell>
                  <TableCell>{new Date(k.added_at).toLocaleString()}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => setTarget(k)}
                    >
                      {t('keys.remove')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <form
          className="grid gap-3 sm:grid-cols-[1fr_2fr_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate(
              { name: name.trim(), public_key: publicKey.trim() },
              {
                onSuccess: () => {
                  toast.add({ type: 'success', title: t('keys.added') })
                  setName('')
                  setPublicKey('')
                },
              },
            )
          }}
        >
          <Field>
            <FieldLabel htmlFor="key-name">{t('keys.name')}</FieldLabel>
            <Input
              id="key-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="key-public">{t('keys.publicKey')}</FieldLabel>
            <Input
              id="key-public"
              value={publicKey}
              onChange={(e) => setPublicKey(e.target.value)}
              spellCheck={false}
              className="font-mono text-xs"
            />
          </Field>
          <Button
            type="submit"
            disabled={!name.trim() || !publicKey.trim() || add.isPending}
          >
            {add.isPending ? t('keys.adding') : t('keys.add')}
          </Button>
        </form>
        {add.error ? (
          <p role="alert" className="text-sm text-destructive">
            {add.error.message}
          </p>
        ) : null}
      </CardContent>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={t('keys.removeTitle', { name: target?.name ?? '' })}
        description={t('keys.removeDescription')}
        confirmLabel={t('keys.remove')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() => {
          if (!target) return
          remove.mutate(target.key_id, {
            onSuccess: () => {
              toast.add({ type: 'success', title: t('keys.removed') })
              setTarget(null)
            },
          })
        }}
      />
    </Card>
  )
}
