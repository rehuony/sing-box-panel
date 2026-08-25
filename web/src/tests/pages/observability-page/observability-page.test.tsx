import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { ApiClientProvider } from '@/api/api-client-context';
import { ObservabilityPage } from '@/pages/observability-page';
import { createMockApiClient } from '@/tests/api/mock-api-client';

describe('ObservabilityPage', () => {
  it('refreshes logs and collector evidence without mutating runtime state', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><ObservabilityPage /></ApiClientProvider>);

    await screen.findByText('The exact core process passed its health check.');
    await user.click(screen.getByRole('button', { name: 'Refresh evidence' }));

    expect(client.getMetrics).toHaveBeenCalledTimes(2);
    expect(client.getTrafficStatus).toHaveBeenCalledTimes(2);
    expect(client.listLogs).toHaveBeenCalledTimes(2);
    expect(client.activateStartupArtifact).not.toHaveBeenCalled();
  });
});
