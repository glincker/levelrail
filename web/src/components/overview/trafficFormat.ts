const SECONDS_PER_MINUTE = 60
// Below one request per 10 s, req/s rounds to 0.0x, so show per minute.
const PER_MINUTE_BELOW_RATE = 0.1

/** Value and unit for a rate; tiny rates read per minute instead of 0.00. */
export function formatRate(rate: number): { value: string; unit: string } {
  if (rate > 0 && rate < PER_MINUTE_BELOW_RATE) {
    const perMin = rate * SECONDS_PER_MINUTE
    return {
      value: perMin < 10 ? perMin.toFixed(1) : perMin.toFixed(0),
      unit: 'req/min',
    }
  }
  return {
    value: rate < 10 ? rate.toFixed(2) : rate.toFixed(1),
    unit: 'req/s',
  }
}
