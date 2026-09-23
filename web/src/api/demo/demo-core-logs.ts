import type { ApiClient, CoreLogChunk } from '../api-client';

export function createDemoCoreLogs() {
  const name = `${new Date().toISOString().slice(0, 10)}-000.log`;
  const archiveName = `${new Date(Date.now() - 86_400_000).toISOString().slice(0, 10)}-000.log`;
  let archiveExists = true;
  let archiveText = 'INFO previous run started\nWARN dns: upstream query timed out\nINFO previous run stopped\n';
  let text = [
    'INFO sing-box started (0.21s)',
    'TRACE router: match connection to api.example.com:443',
    'DEBUG dns: exchanged api.example.com A 192.0.2.20',
    'INFO inbound/anytls[edge-in]: inbound connection to api.example.com:443',
    'WARN dns: upstream query timed out, retrying with fallback server',
    'ERROR inbound/anytls[edge-in]: TLS handshake: EOF',
  ]
    .map((line) => `+0000 ${new Date().toISOString().replace('T', ' ').slice(0, 19)} ${line}\n`)
    .join('');
  let sequence = 0;
  const generations = new Map([[name, 'demo-current'], [archiveName, 'demo-archive']]);
  const chunk = (file: string, offset = -1, generation?: string): CoreLogChunk => {
    if (file !== name && (file !== archiveName || !archiveExists)) throw new Error('Log file not found');
    const bytes = new TextEncoder().encode(file === name ? text : archiveText);
    const currentGeneration = generations.get(file)!;
    const reset = Boolean(generation && generation !== currentGeneration);
    return {
      file,
      text: new TextDecoder().decode(bytes.slice(reset ? 0 : Math.max(0, offset))),
      size: bytes.length,
      next_offset: bytes.length,
      generation: currentGeneration,
      reset,
    };
  };
  return {
    async listCoreLogFiles() {
      return {
        items: [
          {
            name,
            size: new TextEncoder().encode(text).length,
            updated_at: new Date().toISOString(),
            deletable: false,
          },
          ...(archiveExists
            ? [{
                name: archiveName,
                size: new TextEncoder().encode(archiveText).length,
                updated_at: new Date(Date.now() - 86_400_000).toISOString(),
                deletable: true,
              }]
            : []),
        ],
      };
    },
    async deleteCoreLogFile(file) {
      if (file !== archiveName || !archiveExists) throw new Error('Log file cannot be deleted');
      archiveExists = false;
    },
    async clearCoreLog(file) {
      chunk(file); // Use the same managed-file validation as reads.
      if (file === name) text = '';
      else archiveText = '';
      generations.set(file, `demo-cleared-${++sequence}`);
    },
    async readCoreLog(file, offset, generation?: string) {
      return chunk(file, offset, generation);
    },
    async* streamCoreLog(file, offset = -1, generation?: string, signal?: AbortSignal) {
      let current = chunk(file, offset, generation);
      yield current;
      while (!signal?.aborted) {
        await new Promise<void>((resolve) => {
          let timer: ReturnType<typeof setTimeout>;
          const done = () => {
            clearTimeout(timer);
            signal?.removeEventListener('abort', done);
            resolve();
          };
          timer = setTimeout(done, 2500);
          signal?.addEventListener('abort', done, { once: true });
        });
        if (signal?.aborted) return;
        const at = new Date().toISOString().replace('T', ' ').slice(0, 19);
        text += `+0000 ${at} INFO [${++sequence} 0ms] outbound/direct: outbound connection to api.example.com:443\n`;
        current = chunk(file, current.next_offset, current.generation);
        yield current;
      }
    },
  } satisfies Pick<ApiClient, 'listCoreLogFiles' | 'deleteCoreLogFile' | 'clearCoreLog' | 'readCoreLog' | 'streamCoreLog'>;
}
