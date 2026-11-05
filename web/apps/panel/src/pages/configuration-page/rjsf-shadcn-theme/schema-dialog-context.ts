import { createContext } from 'react';

import type { SchemaDialogGroup } from './schema-dialog-groups';

export const SchemaDialogContext = createContext<{
  active: string;
  groupForField: (key: string) => SchemaDialogGroup;
} | null>(null);
