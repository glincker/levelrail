import { useEffect, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { useBrand } from './useBrand'
import { createCLIPairing, type CLIPairing } from '../queries/signIn'
import { RateLimitError } from '../queries/auth'

export function formatLeft(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const m = Math.floor(total / 60)
  const s = String(total % 60).padStart(2, '0')
  return `${m}:${s}`
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) {
      return
    }
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [active])
  return now
}

// The command carries a single-use pairing code issued only to this
// signed-in session. It lives in component state and nowhere else.
export function useCliPairing() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const brand = useBrand()
  const [pairing, setPairing] = useState<CLIPairing | null>(null)
  const [copied, setCopied] = useState(false)
  const now = useNow(pairing !== null)
  const create = useMutation({
    mutationFn: createCLIPairing,
    onSuccess: (p) => {
      setPairing(p)
      setCopied(false)
    },
    onError: (err) => {
      toast.add({
        title:
          err instanceof RateLimitError
            ? t('connectCli.rateLimited')
            : t('connectCli.failed'),
        type: 'error',
      })
    },
  })
  const left = pairing ? new Date(pairing.expires_at).getTime() - now : 0
  const live = pairing !== null && left > 0
  const command = pairing
    ? `${brand.BinaryName} login --api-url ${pairing.server_url} --code ${pairing.code}`
    : ''
  const copy = () => {
    navigator.clipboard.writeText(command).then(
      () => {
        setCopied(true)
        toast.add({ title: t('connectCli.copied'), type: 'success' })
      },
      () => {
        toast.add({ title: t('connectCli.copyFailed'), type: 'error' })
      },
    )
  }
  return {
    pairing,
    live,
    expired: pairing !== null && !live,
    left,
    command,
    copied,
    copy,
    generate: () => {
      create.mutate()
    },
    pending: create.isPending,
  }
}
