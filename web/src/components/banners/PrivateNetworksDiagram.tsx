import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

const TILES = [
  { x: 140, y: 140, p: GREY },
  { x: 260, y: 95, p: BLUE },
  { x: 380, y: 140, p: GREY },
  { x: 260, y: 175, p: BLUE },
]

export default function PrivateNetworksDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('privateNetworks')
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
        {TILES.flatMap((a, i) =>
          TILES.slice(i + 1).map((b) => (
            <path
              key={`${a.x}-${a.y}-${b.x}-${b.y}`}
              d={`M${a.x} ${a.y + 6}L${b.x} ${b.y + 6}`}
            />
          )),
        )}
      </g>
      {TILES.map((tile) => (
        <Block
          key={`${tile.x}-${tile.y}`}
          x={tile.x}
          y={tile.y}
          a={26}
          h={10}
          p={tile.p}
        />
      ))}
      <Block x={200} y={118} a={7} h={7} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M140 118V106" />
            <path d="M260 198V230" />
          </g>
          <rect
            x="200"
            y="230"
            width="120"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="140" y="98" textAnchor="middle">
              {t('banner.diagram.apps')}
            </text>
            <text
              x="260"
              y="249"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('banner.diagram.mesh')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
