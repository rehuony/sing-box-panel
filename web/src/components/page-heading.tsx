export interface PageHeadingProps {
  title: string;
  eyebrow: string;
  summary: string;
  action?: React.ReactNode;
}

export function PageHeading({
  action,
  eyebrow,
  summary,
  title,
}: PageHeadingProps) {
  return (
    <header className='page-heading'>
      <div>
        <p className='eyebrow'>{eyebrow}</p>
        <h1>{title}</h1>
        <p>{summary}</p>
      </div>
      {action === undefined
        ? null
        : (
            <div className='page-heading__action'>{action}</div>
          )}
    </header>
  );
}
