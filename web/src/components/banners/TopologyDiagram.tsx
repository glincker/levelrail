import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

const LEAVES = [
  { x: 130, y: 95 },
  { x: 130, y: 175 },
  { x: 390, y: 95 },
  { x: 390, y: 175 },
]

export default function TopologyDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('network')
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
        {LEAVES.map((l) => (
          <path key={`${l.x}-${l.y}`} d={`M260 140L${l.x} ${l.y + 6}`} />
        ))}
      </g>
      {LEAVES.map((l) => (
        <Block key={`${l.x}-${l.y}`} x={l.x} y={l.y} a={22} h={10} p={GREY} />
      ))}
      <Block x={260} y={135} a={32} h={12} p={BLUE} />
      <Block x={195} y={118} a={7} h={7} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M260 160V232" strokeDasharray="2 3" />
            <path d="M104 95H112" />
            <path d="M414 95H422" />
          </g>
          <rect
            x="205"
            y="232"
            width="110"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="96" y="100" textAnchor="end">
              {t('banner.diagram.app')}
            </text>
            <text x="426" y="100">
              {t('banner.diagram.node')}
            </text>
            <text
              x="260"
              y="251"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('banner.diagram.server')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
