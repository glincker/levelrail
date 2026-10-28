import type { AppDetail } from '../../types/appDetail'

const GENERATED_KEY = /^SERVICE_(PASSWORD|USER|FQDN)_/

/** A recorded template id marks a template app; older apps fall back to generated service keys or the trial flag. */
export function hasTemplateAccess(app: AppDetail): boolean {
  return (
    (app.template_id ?? '') !== '' ||
    app.is_trial === true ||
    (app.secret_env ?? []).some((key) => GENERATED_KEY.test(key))
  )
}
