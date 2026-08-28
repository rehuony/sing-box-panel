import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';

import { ApiClientProvider } from '@/api/api-client-context';
import { StartupWorkflow } from '@/pages/configuration-page/startup-workflow';
import { createMockApiClient, testArtifacts, testRevision } from '@/tests/api/mock-api-client';

describe('startupWorkflow', () => {
  it('requires explicit acceptance of the exact ignored-fields digest before compiling', async () => {
    const user = userEvent.setup();
    const ignoredDigest = '9'.repeat(64);
    const compileConfiguration = vi.fn().mockRejectedValue(new Error('test stops after request evidence'));
    const client = createMockApiClient({
      previewConfiguration: vi.fn().mockResolvedValue({
        canonical_revision: testRevision,
        core_artifact: testArtifacts.items[0],
        support: {
          supported: true,
          profile: { exact_version: '1.13.19', os: 'linux', arch: 'arm64', variant: 'plain', feature_fingerprint: {} },
          adapter_id: 'sing-box-1.13.19', adapter_revision: '1',
        },
        config: {},
        diagnostics: [{ class: 'ignored', code: 'unsupported_field', path: '/services', message: 'Not supported by this version.' }],
        ignored_digest: ignoredDigest,
      }),
      compileConfiguration,
    });

    render(<ApiClientProvider client={client}><StartupWorkflow exactVersion='1.13.19' /></ApiClientProvider>);

    const compile = await screen.findByRole('button', { name: 'Compile and queue check' });
    expect(compile).toBeDisabled();
    await user.click(screen.getByLabelText(/I understand these fields remain/));
    await user.click(compile);

    expect(compileConfiguration).toHaveBeenCalledWith({
      coreArtifactID: 'core_1',
      acceptedIgnoredDigest: ignoredDigest,
    });
  });
});
