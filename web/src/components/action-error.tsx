import { ErrorNotice } from '@/components/error-notice';

export interface ActionErrorProps {
  title: string;
  message: string;
}

export function ActionError({ message, title }: ActionErrorProps) {
  return <ErrorNotice error={message} title={title} />;
}
