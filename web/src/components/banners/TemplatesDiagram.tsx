import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

const SHELF = [0, 1, 2, 3, 4]
const LIFTED = 2

export default function TemplatesDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('templates')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '40 60 420 170'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <Block x={150} y={150} a={78} h={6} p={GREY} />
      {SHELF.map((i) => {
        const lifted = i === LIFTED
        return (
          <Block
            key={i}
            x={92 + i * 30}
            y={122 + i * 15 - (lifted ? 30 : 0)}
            a={13}
            h={8}
            p={lifted ? YELLOW : BLUE}
          />
        )
      })}
      <path
        d="M184 100L372 168"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
        strokeDasharray="4 4"
      />
      <path
        d="M364 158L374 169L361 172"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
      />
      <Block x={420} y={176} a={44} h={22} p={BLUE} />
      <g className="stroke-card" strokeWidth="1.2" strokeLinecap="round">
        <path d="M388 192L418 207" />
        <path d="M388 200L418 215" />
      </g>
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M150 112V82" />
            <path d="M420 146V120" />
          </g>
          <rect
            x="186"
            y="212"
            width="150"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="150" y="74" textAnchor="middle">
              {t('banner.diagram.catalog')}
            </text>
            <text
              x="261"
              y="231"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('banner.diagram.oneClick')}
            </text>
            <text x="420" y="112" textAnchor="middle">
              {t('banner.diagram.server')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
