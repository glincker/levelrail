import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function ProjectsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('common')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '130 105 280 155'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <Block x={270} y={185} a={110} h={12} p={GREY} />
      <Block x={178} y={150} a={26} h={6} p={GREY} />
      <Block x={234} y={149} a={16} h={18} p={BLUE} />
      <Block x={270} y={167} a={16} h={18} p={YELLOW} />
      <Block x={306} y={185} a={16} h={18} p={BLUE} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M270 132V112" />
          </g>
          <g className={LABEL} textAnchor="middle">
            <text x="270" y="104">
              {t('banners.projects.diagram.project')}
            </text>
            <text x="270" y="280">
              {t('banners.projects.diagram.members')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
