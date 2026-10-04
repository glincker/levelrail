import { useEffect, useState } from 'react'
import {
  BellRingingIcon,
  CheckCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  useCreatePushSubscription,
  usePushSubscriptions,
  usePushVAPIDPublicKey,
} from '../queries/pushSubscriptions'

// urlBase64ToUint8Array converts the VAPID public key (base64url, as
// internal/webpush.EnsureVAPIDKeys produces it) into the raw bytes
// PushManager.subscribe's applicationServerKey option requires.
function urlBase64ToUint8Array(base64url: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64url.length % 4)) % 4)
  const base64 = (base64url + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = window.atob(base64)
  const bytes = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) {
    bytes[i] = raw.charCodeAt(i)
  }
  return bytes
}

// subscriptionEndpointsMatch compares the browser's current push
// subscription (if any) against the server's own registered list, so
// "already enabled on this browser" survives a page reload without
// re-prompting for permission.
async function currentBrowserEndpoint(): Promise<string | null> {
  if (!('serviceWorker' in navigator)) {
    return null
  }
  const registration =
    await navigator.serviceWorker.getRegistration('/push-sw.js')
  if (!registration) {
    return null
  }
  const subscription = await registration.pushManager.getSubscription()
  return subscription?.endpoint ?? null
}

// BrowserPushField is the "webpush" channel kind's destination field:
// unlike every other kind's single text input, enabling push is a
// multi-step browser action (permission, service worker, subscribe),
// so this owns that whole flow and reports back through onReadyChange
// instead of exposing a notifyUrl string the parent form can collect.
export function BrowserPushField({
  onReadyChange,
}: {
  onReadyChange: (ready: boolean) => void
}) {
  const { data: publicKey, isError: publicKeyError } = usePushVAPIDPublicKey()
  const { data: subscriptions } = usePushSubscriptions(Boolean(publicKey))
  const createSubscription = useCreatePushSubscription()
  const [browserEndpoint, setBrowserEndpoint] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void currentBrowserEndpoint().then(setBrowserEndpoint)
  }, [])

  const alreadyRegistered = Boolean(
    browserEndpoint &&
    subscriptions?.some((s) => s.user_agent === navigator.userAgent) &&
    subscriptions.length > 0,
  )
  const ready = alreadyRegistered || createSubscription.isSuccess

  useEffect(() => {
    onReadyChange(ready)
  }, [ready, onReadyChange])

  async function handleEnable() {
    setError(null)
    try {
      if (Notification.permission === 'denied') {
        throw new Error(
          'Notifications are blocked for this site in your browser settings.',
        )
      }
      if (Notification.permission === 'default') {
        const permission = await Notification.requestPermission()
        if (permission !== 'granted') {
          throw new Error('Notification permission was not granted.')
        }
      }
      if (!publicKey) {
        throw new Error('Browser push is not configured on this control plane.')
      }

      const registration = await navigator.serviceWorker.register('/push-sw.js')
      await navigator.serviceWorker.ready
      let subscription = await registration.pushManager.getSubscription()
      if (!subscription) {
        subscription = await registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(publicKey),
        })
      }
      const json = subscription.toJSON()
      if (!json.endpoint || !json.keys?.p256dh || !json.keys.auth) {
        throw new Error('Browser did not return a usable push subscription.')
      }

      await createSubscription.mutateAsync({
        endpoint: json.endpoint,
        keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
        user_agent: navigator.userAgent,
      })
      setBrowserEndpoint(json.endpoint)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <Field>
      <FieldLabel>Browser push</FieldLabel>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={ready || createSubscription.isPending || publicKeyError}
          onClick={() => {
            void handleEnable()
          }}
        >
          <BellRingingIcon className="size-3.5" aria-hidden="true" />
          {ready
            ? 'Enabled on this browser'
            : createSubscription.isPending
              ? 'Enabling...'
              : 'Enable browser push'}
        </Button>
        {ready ? (
          <span className="flex items-center gap-1 text-xs font-medium text-emerald-600 dark:text-emerald-400">
            <CheckCircleIcon className="size-3.5" aria-hidden="true" />
            Ready
          </span>
        ) : null}
      </div>
      {publicKeyError ? (
        <span className="flex items-center gap-1 text-xs text-destructive">
          <WarningIcon className="size-3.5" aria-hidden="true" />
          Browser push is not configured on this control plane (no master key
          set).
        </span>
      ) : null}
      {error ? (
        <span className="flex items-center gap-1 text-xs text-destructive">
          <WarningIcon className="size-3.5" aria-hidden="true" />
          {error}
        </span>
      ) : null}
      <FieldHint>
        Notifies this browser even when no dashboard tab is open or focused.
        Works only over HTTPS (or localhost).
      </FieldHint>
    </Field>
  )
}
