import { cn } from '@/lib/utils';
import { Spinner } from '@/components/ui/spinner';

export function LoadingState({ label, fullScreen = true, className }: {
  label: string;
  fullScreen?: boolean;
  className?: string;
}) {
  const Container = fullScreen ? 'main' : 'div';
  return (
    <Container
      className={cn('loading-screen', !fullScreen && 'loading-screen--inline', className)}
      aria-busy='true'
    >
      <div className='loading-screen__content' role='status' aria-label={label}>
        <span aria-hidden='true' className='loading-screen__mark'>
          <Spinner className='loading-screen__spinner' role={undefined} strokeWidth={1.5} />
        </span>
        <p>{label}</p>
      </div>
    </Container>
  );
}
