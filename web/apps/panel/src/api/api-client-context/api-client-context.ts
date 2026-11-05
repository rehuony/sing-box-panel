import { createContext } from 'react';

import type { ApiClient } from '../api-client';

export const ApiClientContext = createContext<ApiClient | null>(null);
