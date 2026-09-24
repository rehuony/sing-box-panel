export function LoadingState({ label, fullScreen = true }: { label: string; fullScreen?: boolean }) {
  const Container = fullScreen ? 'main' : 'div';
  return (
    <Container
      className={fullScreen ? 'loading-screen' : 'loading-screen loading-screen--inline'}
      aria-busy='true'
      aria-live='polite'
    >
      <span aria-hidden='true' className='loading-screen__mark' />
      <p>{label}</p>
    </Container>
  );
}
