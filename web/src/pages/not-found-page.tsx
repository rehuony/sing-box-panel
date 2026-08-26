import { Link } from 'react-router-dom';

export function NotFoundPage() {
  return (
    <div className='load-error not-found-page'>
      <p className='eyebrow'>Route unavailable</p>
      <h1>This console area does not exist.</h1>
      <p>Use the control-plane navigation to return to a supported operation.</p>
      <Link className='button button--primary' to='/'>
        Return to overview
      </Link>
    </div>
  );
}
