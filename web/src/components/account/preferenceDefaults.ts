import type { Preferences } from '../../queries/accountSelf'

// Empty values mean "follow the browser"; reset writes exactly these.
export const DEFAULT_PREFERENCES: Preferences = {
  timezone: '',
  language: '',
  theme: '',
  landing_page: '',
  gravatar: false,
}
