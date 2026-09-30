// A fingerprint is an optional abuse signal, not proof of identity. Keep only
// the visitor ID in page memory and never transmit components or usage telemetry.
let pending: Promise<string | undefined> | undefined;

export function loginFingerprint(): Promise<string | undefined> {
  if (typeof window === 'undefined') return Promise.resolve(undefined);
  pending ??= collectFingerprint();
  return pending;
}

async function collectFingerprint(): Promise<string | undefined> {
  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      import('@fingerprintjs/fingerprintjs').then(async ({ load }) => {
        const agent = await load({ monitoring: false });
        const result = await agent.get();
        return /^[a-f0-9]{32}$/.test(result.visitorId) ? result.visitorId : undefined;
      }).catch(() => undefined),
      new Promise<undefined>(resolve => {
        timeout = setTimeout(resolve, 1500, undefined);
      }),
    ]);
  } finally {
    clearTimeout(timeout);
  }
}
