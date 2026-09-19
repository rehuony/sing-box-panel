import type { ApiClient, CoreLogChunk } from '../api-client';

export function createDemoCoreLogs() {
  const name = `${new Date().toISOString().slice(0, 10)}-000.log`;
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
  const chunk = (offset = -1): CoreLogChunk => {
    const bytes = new TextEncoder().encode(text);
    return {
      file: name,
      text: new TextDecoder().decode(bytes.slice(Math.max(0, offset))),
      size: bytes.length,
      next_offset: bytes.length,
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
          },
        ],
      };
    },
    async readCoreLog(_file, offset) {
      return chunk(offset);
    },
    async* streamCoreLog(_file, offset = -1, signal) {
      let current = chunk(offset);
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
        current = chunk(current.next_offset);
        yield current;
      }
    },
  } satisfies Pick<ApiClient, 'listCoreLogFiles' | 'readCoreLog' | 'streamCoreLog'>;
}
