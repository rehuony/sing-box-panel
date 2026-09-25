import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';

import type { SubscriptionToken } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

import { buildPublicSubscriptionURL } from './public-subscription-url';

function useSubscriptionTokens() {
  const client = useApiClient();
  const [result, setResult] = useState<{
    tokens: SubscriptionToken[];
    loading: boolean;
    error?: unknown;
  }>({ tokens: [], loading: true });
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      const tokens: SubscriptionToken[] = [];
      let cursor: { id: string; created_at: string } | undefined;
      do {
        const page = await client.listSubscriptionTokens(
          { limit: 100, beforeID: cursor?.id, beforeTime: cursor?.created_at }, controller.signal,
        );
        tokens.push(...page.items);
        cursor = page.next;
      } while (cursor && !controller.signal.aborted);
      if (!controller.signal.aborted) setResult({ tokens, loading: false });
    }
    void load().catch((error) => {
      if (!controller.signal.aborted) setResult({ tokens: [], loading: false, error });
    });
    return () => controller.abort();
  }, [client]);
  return result;
}

export function ChannelLinkDialog({ channelID, onClose }: {
  channelID: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const { tokens, loading, error } = useSubscriptionTokens();
  const [selected, setSelected] = useState('');
  const [busy, setBusy] = useState(false);
  const lifetimeRef = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    lifetimeRef.current = controller;
    return () => controller.abort();
  }, []);
  const available = tokens.filter((token) => token.active);
  const selectedID = available.some((token) => token.id === selected) ? selected : available[0]?.id ?? '';
  async function copy() {
    const controller = lifetimeRef.current;
    if (busy || !selectedID || !controller || controller.signal.aborted) return;
    setBusy(true);
    try {
      const [channel, token] = await Promise.all([
        client.getSubscriptionChannel(channelID, controller.signal),
        client.getSubscriptionToken(selectedID, controller.signal),
      ]);
      if (!channel.enabled) throw new Error(t('channels.channelUnavailable'));
      if (!token.active) throw new Error(t('channels.keyUnavailable'));
      const secret = await client.getSubscriptionTokenSecret(selectedID, controller.signal);
      if (controller.signal.aborted) return;
      await navigator.clipboard.writeText(buildPublicSubscriptionURL(secret.token, channelID));
      if (!controller.signal.aborted) toast.add({ title: t('channels.copied'), type: 'success' });
    } catch (reason) {
      if (!controller.signal.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='subscription-source-dialog'>
        <DialogHeader>
          <DialogTitle>{t('channels.copyURL')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.subscriptionKey')}</DialogDescription>
        </DialogHeader>
        {error != null && <ErrorNotice error={error} />}
        <div className='subscription-settings-fields'>
          <label htmlFor='channel-link-key'>{t('channels.subscriptionKey')}</label>
          <SelectField id='channel-link-key' value={selectedID} onValueChange={setSelected} disabled={loading || busy || !available.length}
            items={available.map((token) => ({ value: token.id, label: token.label }))} />
        </div>
        {!loading && !error && !available.length && <p className='text-sm text-muted-foreground'>{t('channels.noActiveKeys')}</p>}
        <DialogFooter>
          <Button disabled={busy || loading || !selectedID} onClick={() => void copy()}>{t('channels.copy')}</Button>
          <Button variant='outline' onClick={onClose}>{t('channels.done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
