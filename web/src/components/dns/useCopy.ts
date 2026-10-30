import { useState } from 'react'
import { COPY_FEEDBACK_MS } from '@/lib/copyFeedback'

export function useCopy() {
  const [copied, setCopied] = useState<string | null>(null)
  const copy = (id: string, text: string) => {
    void navigator.clipboard?.writeText(text).then(() => {
      setCopied(id)
      window.setTimeout(() => setCopied(null), COPY_FEEDBACK_MS)
    })
  }
  return { copied, copy }
}
