interface CachedRead {
  readers: number;
  settled: boolean;
  expiresAt: number;
  promise: Promise<unknown>;
  controller: AbortController;
}

// Owned by one HTTP client; never persisted or shared between sessions.
export function createReadCache() {
  const entries = new Map<string, CachedRead>();

  return {
    clear() {
      // Existing readers may finish, but their results cannot repopulate the cache.
      entries.clear();
    },
    async read<T>(
      key: string,
      load: (signal: AbortSignal) => Promise<T>,
      maxAge: number,
      signal?: AbortSignal | null,
    ): Promise<T> {
      signal?.throwIfAborted();
      let entry = entries.get(key);
      if (entry?.settled && entry.expiresAt <= Date.now()) {
        entries.delete(key);
        entry = undefined;
      }
      if (!entry) {
        const controller = new AbortController();
        const pending: CachedRead = {
          controller,
          promise: Promise.resolve().then(() => load(controller.signal)),
          expiresAt: 0,
          settled: false,
          readers: 0,
        };
        pending.promise = pending.promise.then(value => {
          pending.settled = true;
          pending.expiresAt = Date.now() + maxAge;
          if (maxAge === 0 && entries.get(key) === pending) entries.delete(key);
          return value;
        }, error => {
          pending.settled = true;
          if (entries.get(key) === pending) entries.delete(key);
          throw error;
        });
        // Pagination and artifact identities must not grow this map without bound.
        if (entries.size >= 100) entries.delete(entries.keys().next().value!);
        entries.set(key, pending);
        entry = pending;
      }
      const current = entry;
      current.readers += 1;
      return new Promise<T>((resolve, reject) => {
        let done = false;
        function finish() {
          if (done) return false;
          done = true;
          signal?.removeEventListener('abort', abort);
          current.readers -= 1;
          if (current.readers === 0 && !current.settled) {
            if (entries.get(key) === current) entries.delete(key);
            current.controller.abort();
          }
          return true;
        }
        function abort() {
          if (finish()) reject(signal?.reason);
        }
        signal?.addEventListener('abort', abort, { once: true });
        current.promise.then(value => {
          if (finish()) resolve(structuredClone(value) as T);
        }, error => {
          if (finish()) reject(error);
        });
      });
    },
  };
}
