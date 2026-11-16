import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { BUILD_QUEUE_REASON_MAX } from '../../queries/buildQueue'

// Pause asks for an optional reason so teammates see why builds are waiting.
export function PauseForm({
  pending,
  onSubmit,
}: {
  pending: boolean
  onSubmit: (reason: string) => void
}) {
  const { t } = useTranslation('deploys')
  const id = useId()
  const [reason, setReason] = useState('')
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(reason)
      }}
    >
      <Label htmlFor={id}>{t('buildQueue.pause.reasonLabel')}</Label>
      <Input
        id={id}
        value={reason}
        maxLength={BUILD_QUEUE_REASON_MAX}
        placeholder={t('buildQueue.pause.reasonPlaceholder')}
        autoComplete="off"
        onChange={(e) => {
          setReason(e.target.value)
        }}
      />
      <Button type="submit" size="sm" writeAction disabled={pending}>
        {t('buildQueue.pause.submit')}
      </Button>
    </form>
  )
}
