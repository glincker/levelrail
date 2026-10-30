import { useTranslation } from 'react-i18next'

// Intro line for MySQL and MariaDB, whose point-in-time restore archives the
// binary log rather than WAL.
export function PITRBinlogCopy() {
  const { t } = useTranslation('databases')
  return <>{t('pitrNew.enableBinlog')}</>
}
