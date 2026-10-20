// Minimal service worker for browser push notifications. Registered by
// BrowserPushField.tsx, scoped to "/" only to receive push events and
// show a Notification; it does not cache anything or intercept fetch,
// so it never interferes with the dashboard's own normal loading.

self.addEventListener('install', () => {
  // Activate immediately: there is no previous version of this worker
  // to keep serving old content for, unlike a caching service worker.
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

self.addEventListener('push', (event) => {
  let payload = { title: 'Levelrail', body: '' }
  try {
    if (event.data) {
      payload = event.data.json()
    }
  } catch {
    // A push with no JSON body (or a malformed one) still shows a
    // generic notification rather than silently dropping the event.
  }

  event.waitUntil(
    self.registration.showNotification(payload.title || 'Levelrail', {
      body: payload.body || '',
      icon: '/apple-touch-icon.png',
      badge: '/apple-touch-icon.png',
      tag: 'levelrail-notification',
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  event.waitUntil(
    self.clients
      .matchAll({ type: 'window', includeUncontrolled: true })
      .then((windows) => {
        for (const client of windows) {
          if ('focus' in client) {
            return client.focus()
          }
        }
        if (self.clients.openWindow) {
          return self.clients.openWindow('/')
        }
        return undefined
      }),
  )
})
