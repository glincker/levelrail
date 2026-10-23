import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/** Rename one saved board; the name is the only field changed. */
export function RenameBoardDialog({
  name,
  busy,
  onOpenChange,
  onSave,
}: {
  name: string
  busy: boolean
  onOpenChange: (open: boolean) => void
  onSave: (name: string) => void
}) {
  const { t } = useTranslation('dashboard')
  const [value, setValue] = useState(name)
  const next = value.trim()
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (next) onSave(next)
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('manage.renameTitle')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-1.5">
            <Label htmlFor="board-name">{t('manage.name')}</Label>
            <Input
              id="board-name"
              value={value}
              autoFocus
              onChange={(e) => {
                setValue(e.target.value)
              }}
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                onOpenChange(false)
              }}
            >
              {t('manage.cancel')}
            </Button>
            <Button type="submit" disabled={busy || !next || next === name}>
              {t('manage.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
