import { useEffect, useRef } from 'react'
import {
  useDetectFramework,
  type DetectFrameworkResult,
} from '../queries/buildDetect'

// How long a repo URL/ref pair must sit unchanged before firing
// POST /api/v1/build/detect: Repository URL/Branch stay directly
// editable after a GitRepoSourcePicker pick, so without a debounce
// every keystroke of a hand-edit would fire its own request.
const DETECT_DEBOUNCE_MS = 500

// useAutoDetectFramework fires POST /api/v1/build/detect once repoUrl
// and ref have both settled (DETECT_DEBOUNCE_MS of no further change),
// keyed on that exact pair so switching back to a combination already
// checked this session doesn't re-fire. Not tied to a "Load branches"
// button: a caller who already knows their branch (skipping that picker
// entirely) still gets a detection result. Extracted out of
// GitBuildSourceFields.tsx so that component stays under this repo's
// per-file line budget.
export function useAutoDetectFramework({
  enabled,
  repoUrl,
  ref,
  onDetected,
}: {
  /** false for buildType "image": nothing to detect against. */
  enabled: boolean
  repoUrl: string
  ref: string
  /** Called every time detection settles (success or "nothing
   *  detected"), or with null once repoUrl/ref no longer both resolve
   *  to a real pair worth detecting against. */
  onDetected: (result: DetectFrameworkResult | null) => void
}) {
  const detectMutation = useDetectFramework()
  const detectedForRef = useRef<string | null>(null)

  useEffect(() => {
    if (!enabled || !repoUrl || !ref) {
      onDetected(null)
      return
    }
    const key = `${repoUrl}@${ref}`
    if (detectedForRef.current === key) return

    const timer = setTimeout(() => {
      detectedForRef.current = key
      detectMutation.mutate(
        { repoUrl, ref },
        {
          onSuccess: (result) => {
            onDetected(result)
          },
          onError: () => {
            // Detection is a pre-flight nicety, never a blocker: a failed
            // call (network hiccup, unusual repo) just leaves the manual
            // build-type picker exactly as it already was.
            onDetected(null)
          },
        },
      )
    }, DETECT_DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
    }
    // onDetected is a fresh closure every render on the caller's side
    // but is only ever invoked from inside this effect's own mutate
    // callbacks, so it doesn't need to be a dependency for this effect
    // to stay correct.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, repoUrl, ref])

  return { isPending: detectMutation.isPending, result: detectMutation.data }
}
