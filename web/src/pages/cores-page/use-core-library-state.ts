import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type {
  CatalogAssetList,
  ConfigurationSupport,
  CoreArtifact,
  CoreArtifactCursor,
  CoreArtifactPage,
  StartupArtifactSummary,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

export interface CoreArtifactInspection {
  startupError?: unknown;
  artifact?: CoreArtifact;
  artifactError?: unknown;
  startupArtifacts?: StartupArtifactSummary[];
}

interface SchemaSupportResult {
  error?: unknown;
  artifactID: string;
  support?: ConfigurationSupport;
}

export function useCoreLibraryState(selectedVersion: string) {
  const client = useApiClient();
  const [catalog, setCatalog] = useState<CatalogAssetList | null>(null);
  const [artifacts, setArtifacts] = useState<CoreArtifactPage | null>(null);
  const [catalogError, setCatalogError] = useState<unknown>(null);
  const [artifactError, setArtifactError] = useState<unknown>(null);
  const [schemaSupportResult, setSchemaSupportResult] = useState<SchemaSupportResult | null>(null);
  const [inspectionID, setInspectionID] = useState<string | null>(null);
  const [inspection, setInspection] = useState<CoreArtifactInspection | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const catalogControllerRef = useRef<AbortController | null>(null);
  const artifactControllerRef = useRef<AbortController | null>(null);
  const inspectionControllerRef = useRef<AbortController | null>(null);
  const catalogRequestRef = useRef(0);
  const artifactRequestRef = useRef(0);
  const inspectionRequestRef = useRef(0);
  const loadingOlderRef = useRef(false);

  const loadCatalog = useCallback(async () => {
    catalogControllerRef.current?.abort();
    const controller = new AbortController();
    catalogControllerRef.current = controller;
    const request = catalogRequestRef.current + 1;
    catalogRequestRef.current = request;
    setCatalogError(null);
    try {
      const nextCatalog = await client.listCatalogAssets({}, controller.signal);
      if (!controller.signal.aborted && catalogRequestRef.current === request) setCatalog(nextCatalog);
    } catch (error) {
      if (!controller.signal.aborted && catalogRequestRef.current === request) {
        setCatalog(null);
        setCatalogError(error);
      }
    }
  }, [client]);

  const loadArtifacts = useCallback(async (cursor?: CoreArtifactCursor, append = false) => {
    artifactControllerRef.current?.abort();
    const controller = new AbortController();
    artifactControllerRef.current = controller;
    const request = artifactRequestRef.current + 1;
    artifactRequestRef.current = request;
    setArtifactError(null);
    try {
      const page = await client.listCoreArtifacts({
        beforeID: cursor?.id,
        beforeTime: cursor?.created_at,
        limit: 200,
      }, controller.signal);
      if (controller.signal.aborted || artifactRequestRef.current !== request) return;
      setArtifacts((current) => append && current !== null
        ? { items: [...current.items, ...page.items], next: page.next }
        : page);
    } catch (error) {
      if (!controller.signal.aborted && artifactRequestRef.current === request) {
        if (!append) setArtifacts(null);
        setArtifactError(error);
      }
    }
  }, [client]);

  useEffect(() => {
    void loadCatalog();
    void loadArtifacts();
    return () => {
      catalogControllerRef.current?.abort();
      artifactControllerRef.current?.abort();
      inspectionControllerRef.current?.abort();
    };
  }, [loadArtifacts, loadCatalog]);

  const versionArtifacts = useMemo(
    () => (artifacts?.items ?? []).filter((artifact) => artifact.exact_version === selectedVersion),
    [artifacts, selectedVersion],
  );
  const schemaArtifact = versionArtifacts.find((artifact) => artifact.verification_state === 'verified');
  const currentSchemaSupportResult = schemaSupportResult?.artifactID === schemaArtifact?.id
    ? schemaSupportResult
    : null;

  useEffect(() => {
    if (schemaArtifact === undefined) return undefined;
    const controller = new AbortController();
    void client.getConfigurationSupport(schemaArtifact.id, controller.signal)
      .then((support) => {
        if (!controller.signal.aborted) setSchemaSupportResult({ artifactID: schemaArtifact.id, support });
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setSchemaSupportResult({ artifactID: schemaArtifact.id, error });
      });
    return () => controller.abort();
  }, [client, schemaArtifact]);

  const inspectArtifact = useCallback(async (artifact: CoreArtifact) => {
    inspectionControllerRef.current?.abort();
    const controller = new AbortController();
    inspectionControllerRef.current = controller;
    const request = inspectionRequestRef.current + 1;
    inspectionRequestRef.current = request;
    setInspectionID(artifact.id);
    setInspection(null);
    const [artifactResult, startupResult] = await Promise.allSettled([
      client.getCoreArtifact(artifact.id, controller.signal),
      client.listStartupArtifacts({ coreArtifactID: artifact.id, limit: 100 }, controller.signal),
    ]);
    if (
      controller.signal.aborted
      || inspectionControllerRef.current !== controller
      || inspectionRequestRef.current !== request
    ) {
      return;
    }
    setInspection({
      artifact: artifactResult.status === 'fulfilled' ? artifactResult.value : undefined,
      artifactError: artifactResult.status === 'rejected' ? artifactResult.reason : undefined,
      startupArtifacts: startupResult.status === 'fulfilled' ? startupResult.value.items : undefined,
      startupError: startupResult.status === 'rejected' ? startupResult.reason : undefined,
    });
  }, [client]);

  const closeInspection = useCallback(() => {
    inspectionRequestRef.current += 1;
    inspectionControllerRef.current?.abort();
    inspectionControllerRef.current = null;
    setInspectionID(null);
    setInspection(null);
  }, []);

  const loadOlder = useCallback(async () => {
    if (artifacts?.next === undefined || loadingOlderRef.current) return;
    loadingOlderRef.current = true;
    setLoadingOlder(true);
    try {
      await loadArtifacts(artifacts.next, true);
    } finally {
      loadingOlderRef.current = false;
      setLoadingOlder(false);
    }
  }, [artifacts?.next, loadArtifacts]);

  return {
    schemaArtifact,
    schemaError: currentSchemaSupportResult?.error,
    schemaSupport: currentSchemaSupportResult?.support,
    artifactError,
    artifacts,
    catalog,
    catalogError,
    closeInspection,
    inspectArtifact,
    inspection,
    inspectionID,
    loadArtifacts,
    loadCatalog,
    loadOlder,
    loadingOlder,
    versionArtifacts,
  };
}
