import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { GREY, LABEL, YELLOW } from './isoTokens'

export default function StatusDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('attention')
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
        <path d="M170 104V150" />
        <path d="M350 104V150" />
      </g>
      <Block x={260} y={165} a={54} h={10} p={GREY} />
      <Block x={238} y={154} a={5} h={3} p={YELLOW} />
      <Block x={260} y={165} a={5} h={3} p={YELLOW} />
      <Block x={282} y={176} a={5} h={3} p={YELLOW} />
      <path
        d="M130 100H200L214 76L232 124L248 90L260 100H390"
        className="stroke-(--iso-accent-top)"
        strokeWidth="2"
        strokeLinejoin="round"
      />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M260 204V222" strokeDasharray="2 3" />
          </g>
          <rect
            x="205"
            y="222"
            width="110"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="130" y="64">
              {t('statusPage.banner.diagram.heartbeat')}
            </text>
            <text
              x="260"
              y="241"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('statusPage.banner.diagram.checks')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
