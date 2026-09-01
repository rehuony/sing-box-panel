import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

export function NotFoundPage() {
  const { t } = useTranslation();

  return (
    <div className='load-error not-found-page'>
      <p className='eyebrow'>{t('notFound.eyebrow')}</p>
      <h1>{t('notFound.title')}</h1>
      <p>{t('notFound.description')}</p>
      <Link className='button button--primary' to='/'>
        {t('notFound.return')}
      </Link>
    </div>
  );
}
