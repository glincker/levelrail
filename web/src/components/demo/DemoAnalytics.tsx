import { useEffect } from 'react'
import { useDemoInfo } from '@/hooks/useDemo'
import { initDemoAnalytics } from '@/lib/demoAnalytics'

/** DemoAnalytics starts demo analytics on every route, including the signed-out login page; it renders nothing. */
export function DemoAnalytics() {
  const info = useDemoInfo()
  useEffect(() => {
    if (info.enabled) void initDemoAnalytics(info)
  }, [info])
  return null
}
