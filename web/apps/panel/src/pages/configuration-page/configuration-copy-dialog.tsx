import { useState } from 'react';
import { Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Button } from '@shared/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { LoadingState } from '@/components/loading-state';
import { useCurrentNode } from '@/stores/current-node.store';
import { describeRequestError } from '@/components/error-notice';

import { ConfigurationDiff } from './configuration-diff';

/** The source the dialog compares; nothing is chosen when it opens. */
const NO_SOURCE = '';

interface ConfigurationCopyProps {
  /** The editor's draft, which the copy replaces. */
  draft: string;
  disabled: boolean;
  onReplace: (content: string) => void;
}

/**
 * Copies another node's saved configuration into this node's editor: pick the
 * source, see what replacing the draft changes, then replace it. Nothing is
 * saved; the draft stays unsaved until it is.
 */
export function ConfigurationCopyDialog({ disabled, draft, onReplace }: ConfigurationCopyProps) {
  const { t } = useTranslation();
  const client = useApiClient();
  const node = useCurrentNode();
  const [open, setOpen] = useState(false);
  const [source, setSource] = useState(NO_SOURCE);
  const nodes = useQuery({ ...queries.fleetNodes(client), enabled: open });
  const others = (nodes.data?.items ?? []).filter(item => item.id !== node.id);
  const file = useQuery({
    enabled: open && source !== NO_SOURCE,
    gcTime: 0,
    queryFn: ({ signal }) => client.forNode(source).getConfigurationFile(signal),
    queryKey: ['configurationCopy', source],
  });
  const content = file.data?.content ?? null;

  let body;
  if (source === NO_SOURCE) {
    body = <p className='m-auto text-sm text-muted-foreground'>{t('configuration.copy.choose')}</p>;
  } else if (file.isPending) {
    body = <LoadingState label={t('configuration.copy.loading')} fullScreen={false} />;
  } else if (content === null) {
    body = <p className='m-auto text-sm text-[var(--color-danger-text)]'>{describeRequestError(file.error)}</p>;
  } else if (content === draft) {
    body = <p className='m-auto text-sm text-muted-foreground'>{t('configuration.copy.same')}</p>;
  } else {
    body = <ConfigurationDiff next={content} original={draft} />;
  }

  return (
    <>
      <Button
        disabled={disabled}
        onClick={() => {
          setSource(NO_SOURCE);
          setOpen(true);
        }}
        size='sm'
        type='button'
        variant='flat'
      >
        <Copy aria-hidden='true' data-icon='inline-start' />
        <span className='max-sm:sr-only'>{t('configuration.copy.action')}</span>
      </Button>
      <Dialog onOpenChange={setOpen} open={open}>
        <DialogContent className='flex h-[min(44rem,calc(100dvh-2rem))] flex-col overflow-hidden' size='workspace'>
          <DialogHeader>
            <DialogTitle>{t('configuration.copy.title', { name: node.name })}</DialogTitle>
            <DialogDescription>{t('configuration.copy.description')}</DialogDescription>
          </DialogHeader>
          <SelectField
            aria-label={t('configuration.copy.source')}
            className='w-full'
            items={[
              { label: t('configuration.copy.sourcePlaceholder'), value: NO_SOURCE },
              ...others.map(item => ({ label: item.name, value: item.id })),
            ]}
            onValueChange={setSource}
            value={source}
          />
          <div className='flex min-h-0 flex-1 flex-col'>{body}</div>
          <DialogFooter>
            <Button onClick={() => setOpen(false)} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button
              onClick={() => {
                if (content === null || content === draft) return;
                onReplace(content);
                setOpen(false);
              }}
              type='button'
            >
              {t('configuration.copy.replace')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
