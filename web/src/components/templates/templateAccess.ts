import type { AppDetail } from '../../types/appDetail'

const GENERATED_KEY = /^SERVICE_(PASSWORD|USER|FQDN)_/

/** The API records no template id on an app, so generated service keys or the trial flag mark a template app. */
export function hasTemplateAccess(app: AppDetail): boolean {
  return (
    app.is_trial === true ||
    (app.secret_env ?? []).some((key) => GENERATED_KEY.test(key))
  )
}
