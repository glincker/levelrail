// Wire types for the browser push subscription resource, matching
// internal/api/push_subscriptions.go's pushSubscriptionResource.

export interface PushSubscription {
  id: string
  user_agent: string
  created_at: string
}

// Mirrors internal/api's createPushSubscriptionRequest: the shape the
// browser's own PushSubscription.toJSON() produces, plus an optional
// user agent string for the settings-page listing.
export interface CreatePushSubscriptionRequest {
  endpoint: string
  keys: {
    p256dh: string
    auth: string
  }
  user_agent?: string
}

export interface PushVAPIDPublicKeyResponse {
  public_key: string
}
