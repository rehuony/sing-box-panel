import type { ReactNode } from 'react';

import { createContext, use, useState } from 'react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';

import { UnsavedChangesProvider } from '@/stores/unsaved-changes-provider';

const ContentContext = createContext<ReactNode>(null);

function TestContent() {
  return <UnsavedChangesProvider>{use(ContentContext)}</UnsavedChangesProvider>;
}

export function TestRouter({ children, ...options }: {
  children: ReactNode;
} & NonNullable<Parameters<typeof createMemoryRouter>[1]>) {
  const [router] = useState(() => createMemoryRouter([{ path: '*', element: <TestContent /> }], options));
  return <ContentContext value={children}><RouterProvider router={router} /></ContentContext>;
}
