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
import { Textarea } from '@/components/ui/textarea'

function parse(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '' && !l.startsWith('#'))
}

/** AiBotListDialog edits the AI crawler list as data: one user agent substring per line, with a reset to the shipped list. */
export function AiBotListDialog({
  open,
  onOpenChange,
  agents,
  defaults,
  max,
  pending,
  onSave,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The custom list; empty means the shipped list is in use. */
  agents: string[]
  defaults: string[]
  max: number
  pending: boolean
  onSave: (agents: string[]) => void
}) {
  const { t } = useTranslation('firewall')
  const [text, setText] = useState(
    (agents.length > 0 ? agents : defaults).join('\n'),
  )
  const parsed = parse(text)
  const tooMany = parsed.length > max

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('bots.listTitle')}</DialogTitle>
          <DialogDescription>{t('bots.listHelp')}</DialogDescription>
        </DialogHeader>
        <Textarea
          aria-label={t('bots.listLabel')}
          aria-invalid={tooMany ? true : undefined}
          rows={12}
          className="font-mono text-xs"
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        <p
          className={
            tooMany
              ? 'text-xs text-destructive'
              : 'text-xs text-muted-foreground'
          }
          role={tooMany ? 'alert' : undefined}
        >
          {tooMany
            ? t('bots.listTooMany', { max })
            : t('bots.listCount', { count: parsed.length })}
        </p>
        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            onClick={() => setText(defaults.join('\n'))}
          >
            {t('bots.listReset')}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('cancel')}
          </Button>
          <Button
            type="button"
            disabled={pending || tooMany}
            onClick={() =>
              onSave(sameAsDefault(parsed, defaults) ? [] : parsed)
            }
          >
            {t('save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function sameAsDefault(list: string[], defaults: string[]): boolean {
  return (
    list.length === defaults.length && list.every((v, i) => v === defaults[i])
  )
}
