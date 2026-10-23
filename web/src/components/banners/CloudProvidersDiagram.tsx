import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

const COLS = [130, 260, 390]

export default function CloudProvidersDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('settings')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '70 55 350 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4" className="stroke-(--iso-l)">
        {COLS.map((x) => (
          <path key={x} d={`M${x} 106V146`} />
        ))}
      </g>
      {COLS.map((x) => (
        <g key={x}>
          <Block x={x} y={88} a={26} h={8} p={GREY} />
          <path
            transform={`translate(${x - 1} 84)`}
            d="M-12 5a6 6 0 0 1 4-11a8 8 0 0 1 15-1a6 6 0 0 1 2 12Z"
            className="fill-card stroke-(--iso-sleep-r)"
            strokeWidth="1.2"
          />
          <Block x={x} y={170} a={34} h={12} p={BLUE} />
          <circle cx={x - 26} cy={174} r="1.8" className="fill-card" />
        </g>
      ))}
      <Block x={260} y={126} a={7} h={7} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M418 90H426" />
            <path d="M426 172H440" />
          </g>
          <g className={LABEL}>
            <text x="430" y="95">
              {t('nodeProvidersPage.banner.diagram.cloud')}
            </text>
            <text x="444" y="177">
              {t('nodeProvidersPage.banner.diagram.node')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
