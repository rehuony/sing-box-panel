import { Navigate, useSearchParams } from 'react-router-dom';

// Keep existing task links working while all product activity shares one page.
export function TasksPage() {
  const [params] = useSearchParams();
  const next = new URLSearchParams();
  next.set('tab', 'panel');
  const task = params.get('task');
  if (task) next.set('task', task);
  return <Navigate replace to={`/observability?${next}`} />;
}
