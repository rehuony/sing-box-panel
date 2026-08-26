import { useEffect, useRef } from 'react';

export interface ActionErrorProps {
  title: string;
  message: string;
}

export function ActionError({ message, title }: ActionErrorProps) {
  const summaryRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    summaryRef.current?.focus();
  }, [message]);

  if (message === '') {
    return null;
  }

  return (
    <div className='form-error' ref={summaryRef} role='alert' tabIndex={-1}>
      <strong>{title}</strong>
      <span>{message}</span>
    </div>
  );
}
