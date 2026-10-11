import { describe, expect, it } from 'vitest'

const sources = import.meta.glob<string>(
  [
    '../components/{MetricChartCard,MetricsDashboard,RequestMetricsSection,DatabaseMetricsDashboard,NodeMetricsDashboard,FleetResourceChart,ModelUsageCard,LoginScreen,CodeLoginForm,NewDeviceApprovalWait}.tsx',
    '../components/resourceAdvisor/EvidenceChart.tsx',
  ],
  { query: '?raw', import: 'default', eager: true },
)

const RAW_COLOR =
  /#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b|rgb\(|\b(amber|green|red|blue|purple)-\d{2,3}\b/

describe('chart and sign-in files use tokens only', () => {
  it('covers every expected file', () => {
    expect(Object.keys(sources)).toHaveLength(11)
  })

  it.each(Object.entries(sources))(
    '%s has no raw hex, rgb or palette colour',
    (_file, src) => {
      expect(RAW_COLOR.test(src)).toBe(false)
    },
  )
})
