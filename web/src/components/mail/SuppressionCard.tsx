import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useAddMailSuppression,
  useMailSuppressions,
  useRemoveMailSuppression,
} from '../../queries/mail'

const VISIBLE = 50

/** SuppressionCard shows the opt-out list that announcements honor, with add and remove. */
export function SuppressionCard() {
  const { t } = useTranslation('mail')
  const { data: list = [] } = useMailSuppressions()
  const add = useAddMailSuppression()
  const remove = useRemoveMailSuppression()
  const [email, setEmail] = useState('')
  const [shown, setShown] = useState(VISIBLE)

  return (
    <Card>
      <CardContent className="space-y-4">
        <div>
          <h2 className="text-sm font-medium">
            {t('audiences.suppressions.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('audiences.suppressions.description')}
          </p>
        </div>
        <form
          className="flex flex-wrap gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate(email.trim(), {
              onSuccess: () => {
                setEmail('')
                toast.add({
                  title: t('audiences.suppressions.added'),
                  type: 'success',
                })
              },
              onError: (error) =>
                toast.add({ title: error.message, type: 'error' }),
            })
          }}
        >
          <Input
            type="email"
            aria-label={t('audiences.suppressions.add')}
            placeholder={t('audiences.suppressions.addPlaceholder')}
            value={email}
            className="max-w-72"
            onChange={(e) => setEmail(e.target.value)}
          />
          <Button
            type="submit"
            size="sm"
            variant="outline"
            disabled={email.trim() === '' || add.isPending}
            writeAction
          >
            {t('audiences.suppressions.add')}
          </Button>
        </form>
        {list.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('audiences.suppressions.empty')}
          </p>
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {list.slice(0, shown).map((s) => (
              <li
                key={s.email}
                className="flex items-center justify-between gap-2 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm">{s.email}</p>
                  <p className="text-xs text-muted-foreground">{s.reason}</p>
                </div>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={`${t('audiences.suppressions.remove')} ${s.email}`}
                  disabled={remove.isPending}
                  writeAction
                  onClick={() => remove.mutate(s.email)}
                >
                  <TrashGlassIcon size="1.4em" />
                </Button>
              </li>
            ))}
          </ul>
        )}
        {list.length > shown ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setShown((n) => n + VISIBLE)}
          >
            +{list.length - shown}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}
