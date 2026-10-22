import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function ImagesDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('images')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '70 55 350 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <path
        d="M290 160L352 168"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
        strokeDasharray="4 4"
      />
      <Block x={370} y={168} a={34} h={12} p={GREY} />
      <Block x={230} y={168} a={48} h={10} p={BLUE} />
      <Block x={230} y={150} a={48} h={10} p={BLUE} />
      <Block x={230} y={132} a={48} h={10} p={BLUE} />
      <Block x={230} y={114} a={48} h={10} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M270 100H300" />
            <path d="M370 192V222" strokeDasharray="2 3" />
          </g>
          <g className={LABEL}>
            <text x="304" y="105">
              {t('banner.diagram.layers')}
            </text>
            <text x="370" y="242" textAnchor="middle">
              {t('banner.diagram.node')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
