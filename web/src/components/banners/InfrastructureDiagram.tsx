import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function InfrastructureDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('settings')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '150 120 280 140'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <Block x={290} y={200} a={80} h={14} p={GREY} />
      <Block x={290} y={178} a={62} h={14} p={BLUE} />
      <Block x={290} y={156} a={44} h={14} p={BLUE} />
      <Block x={290} y={134} a={9} h={9} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M340 163H392" />
            <path d="M358 185H392" />
            <path d="M374 207H392" />
          </g>
          <g className={LABEL}>
            <text x="398" y="168">
              {t('iacPage.banner.diagram.ingress')}
            </text>
            <text x="398" y="190">
              {t('iacPage.banner.diagram.docker')}
            </text>
            <text x="398" y="212">
              {t('iacPage.banner.diagram.host')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
