import type { FormEvent } from 'react';

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@shared/components/ui/alert-dialog';

import type { FleetNode, NodeSettings } from '@/api/api-client';

import { queries } from '@/api/queries';
import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { MAX_TAGS, TagInput } from '@/components/tag-input';
import { useCurrentNode } from '@/stores/current-node.store';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { useEnrollmentDialog } from '@/stores/enrollment-dialog.store';
import { SettingsField, SettingsGroup } from '@/components/settings-field';

interface Draft {
  name: string;
  quota: string;
  months: string;
  resetDay: string;
  publicHost: string;
  tags: readonly string[];
}

type DraftField = keyof Draft;

function draftOf(node: FleetNode, settings: NodeSettings): Draft {
  return {
    name: node.name,
    publicHost: settings.public_host,
    quota: settings.traffic.quota_gib ? String(settings.traffic.quota_gib) : '',
    months: String(settings.traffic.period_months),
    resetDay: String(settings.traffic.reset_day),
    tags: settings.tags,
  };
}

function integer(value: string, min: number, max: number): number | null {
  const parsed = Number(value);
  return value.trim() !== '' && Number.isInteger(parsed) && parsed >= min && parsed <= max ? parsed : null;
}

/** The first field the server cannot accept, checked before saving. */
function invalidField(draft: Draft): DraftField | null {
  const name = draft.name;
  if (name.trim() === '' || name.trim() !== name || [...name].length > 64) return 'name';
  if (draft.publicHost.length > 253 || /[\s/:@?#]/.test(draft.publicHost.replace(/^\[.*\]$/, ''))) return 'publicHost';
  if (draft.quota.trim() !== '' && integer(draft.quota, 0, 8_589_934_591) === null) return 'quota';
  if (integer(draft.months, 1, 120) === null) return 'months';
  if (integer(draft.resetDay, 1, 28) === null) return 'resetDay';
  return null;
}

function SettingsForm({ node, settings }: { node: FleetNode; settings: NodeSettings }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const saved = draftOf(node, settings);
  const [draft, setDraft] = useState(saved);
  const [invalid, setInvalid] = useState<DraftField | null>(null);
  const [message, setMessage] = useState('');
  const nameChanged = draft.name !== node.name;
  const settingsChanged = draft.publicHost !== saved.publicHost || draft.quota !== saved.quota
    || draft.months !== saved.months || draft.resetDay !== saved.resetDay || draft.tags.join() !== saved.tags.join();
  const dirty = nameChanged || settingsChanged;
  const save = useMutation({
    mutationFn: async (value: Draft) => {
      if (settingsChanged) {
        const quota = value.quota.trim() === '' ? null : Number(value.quota);
        await client.saveNodeSettings({
          revision: settings.revision,
          public_host: value.publicHost.trim(),
          traffic: {
            quota_gib: quota === 0 ? null : quota,
            period_months: Number(value.months),
            reset_day: Number(value.resetDay),
          },
          tags: [...value.tags],
        });
      }
      if (nameChanged) await client.renameNode(node.id, value.name);
    },
    onSuccess: () => {
      setMessage('');
      toast.add({ title: t('node.settings.saved'), type: 'success' });
    },
    onError: (error) => {
      setMessage(error instanceof ApiRequestError && error.status === 412
        ? t('node.settings.conflict')
        : describeRequestError(error));
    },
    onSettled: () => Promise.all([
      cache.invalidateQueries({ queryKey: ['nodeSettings', node.id] }),
      cache.invalidateQueries({ queryKey: ['fleetStatus'] }),
    ]),
  });
  useUnsavedChanges(dirty, () => setDraft(saved), save.isPending);

  function update<Field extends DraftField>(field: Field, value: Draft[Field]) {
    setDraft(current => ({ ...current, [field]: value }));
    if (invalid === field) {
      setInvalid(null);
      setMessage('');
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (save.isPending) return;
    if (!dirty) {
      setMessage(t('node.settings.unchanged'));
      return;
    }
    const field = invalidField(draft);
    setInvalid(field);
    if (field !== null) {
      setMessage(t(`node.settings.invalid.${field}`));
      return;
    }
    save.mutate(draft);
  }

  return (
    <form className='flex flex-1 flex-col gap-5 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] p-4 sm:p-6' noValidate onSubmit={submit}>
      <SettingsGroup title={t('node.settings.identity')}>
        <SettingsField id='node-name' invalid={invalid === 'name'} label={t('node.settings.name')}>
          <Input aria-invalid={invalid === 'name' || undefined} autoComplete='off' id='node-name' maxLength={64} onChange={event => update('name', event.target.value)} spellCheck={false} value={draft.name} />
        </SettingsField>
        <SettingsField help={t('node.settings.publicHostHelp')} id='node-public-host' invalid={invalid === 'publicHost'} label={t('node.settings.publicHost')}>
          <Input aria-invalid={invalid === 'publicHost' || undefined} autoComplete='off' className='font-mono' id='node-public-host' maxLength={253} onChange={event => update('publicHost', event.target.value)} placeholder={node.public_ip || 'node.example.com'} spellCheck={false} value={draft.publicHost} />
        </SettingsField>
        <SettingsField id='node-tags' invalid={invalid === 'tags'} label={t('node.settings.tags')}>
          <TagInput
            id='node-tags'
            invalid={invalid === 'tags'}
            onChange={tags => update('tags', tags)}
            onReject={(reason) => {
              setInvalid('tags');
              setMessage(t(`node.settings.invalid.tags.${reason}`, { count: MAX_TAGS }));
            }}
            value={draft.tags}
          />
        </SettingsField>
      </SettingsGroup>
      <SettingsGroup title={t('node.settings.traffic')}>
        <SettingsField help={t('node.settings.quotaHelp')} id='node-quota' invalid={invalid === 'quota'} label={t('node.settings.quota')}>
          <InputGroup>
            <InputGroupInput aria-invalid={invalid === 'quota' || undefined} className='tabular-nums' id='node-quota' inputMode='numeric' min={0} onChange={event => update('quota', event.target.value)} placeholder={t('node.settings.unlimited')} type='number' value={draft.quota} />
            <InputGroupAddon align='inline-end'>GiB</InputGroupAddon>
          </InputGroup>
        </SettingsField>
        <SettingsField id='node-period' invalid={invalid === 'months'} label={t('node.settings.period')}>
          <InputGroup>
            <InputGroupInput aria-invalid={invalid === 'months' || undefined} className='tabular-nums' id='node-period' inputMode='numeric' max={120} min={1} onChange={event => update('months', event.target.value)} type='number' value={draft.months} />
            <InputGroupAddon align='inline-end'>{t('node.settings.months')}</InputGroupAddon>
          </InputGroup>
        </SettingsField>
        <SettingsField help={t('node.settings.resetDayHelp')} id='node-reset-day' invalid={invalid === 'resetDay'} label={t('node.settings.resetDay')}>
          <Input aria-invalid={invalid === 'resetDay' || undefined} className='tabular-nums' id='node-reset-day' inputMode='numeric' max={28} min={1} onChange={event => update('resetDay', event.target.value)} type='number' value={draft.resetDay} />
        </SettingsField>
      </SettingsGroup>
      <footer className='-mx-4 -mb-4 mt-auto flex items-center gap-3 border-t px-4 py-3 sm:-mx-6 sm:-mb-6 sm:px-6'>
        <p className='min-h-5 min-w-0 flex-1 truncate text-sm text-muted-foreground' role='status' title={message || undefined}>{message}</p>
        <Button onClick={() => {
          setDraft(saved);
          setInvalid(null);
          setMessage('');
        }} type='button' variant='outline'>
          {t('node.settings.reset')}
        </Button>
        <Button type='submit'>{t(save.isPending ? 'node.settings.saving' : 'node.settings.save')}</Button>
      </footer>
    </form>
  );
}

/** The node's version against the panel's release, and the action that updates it. */
function VersionRow({ node }: { node: FleetNode }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const update = useMutation({
    mutationFn: () => client.updateNode(),
    onSuccess: (result) => {
      setConfirming(false);
      toast.add({ title: t(`node.version.result.${result.state}`, { name: node.name, version: result.version }), type: 'success' });
    },
    onSettled: () => cache.invalidateQueries({ queryKey: ['fleetStatus'] }),
  });
  const last = node.last_update;
  const detail = last && !last.succeeded
    ? t('node.version.lastFailed', { version: last.version, code: last.code })
    : node.update_available ? t('node.version.available') : t('node.version.current');
  return (
    <section aria-labelledby='node-version-title' className='flex flex-wrap items-center gap-x-3 gap-y-2 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] p-4 sm:px-6'>
      <h2 className='text-sm font-semibold' id='node-version-title'>{t('node.version.title')}</h2>
      <p className='me-auto flex min-w-0 items-baseline gap-2 text-sm'>
        <span className='font-mono'>{node.build_version || '—'}</span>
        <span className='truncate text-muted-foreground' title={detail}>{detail}</span>
      </p>
      {node.update_available && (
        <Button onClick={() => {
          update.reset();
          setConfirming(true);
        }} variant='outline'>
          {t('node.version.update')}
        </Button>
      )}
      <AlertDialog onOpenChange={(open) => {
        if (!open && !update.isPending) setConfirming(false);
      }} open={confirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('node.version.confirmTitle', { name: node.name })}</AlertDialogTitle>
            <AlertDialogDescription>{t('node.version.confirm', { name: node.name, current: node.build_version })}</AlertDialogDescription>
          </AlertDialogHeader>
          {update.error ? <p className='text-sm text-[var(--color-danger-text)]' role='alert'>{describeRequestError(update.error)}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => {
              if (!update.isPending) update.mutate();
            }}>
              {t(update.isPending ? 'node.version.updating' : 'node.version.updateAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}

function DangerZone({ node }: { node: FleetNode }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const navigate = useNavigate();
  const [confirm, setConfirm] = useState<'reenroll' | 'remove' | null>(null);
  const reenroll = useMutation({
    mutationFn: () => client.createNodeEnrollment(node.id),
    onSuccess: (enrollment) => {
      setConfirm(null);
      useEnrollmentDialog.getState().reenroll({ enrollment, enrolledBefore: node.enrolled_at });
    },
  });
  const remove = useMutation({
    mutationFn: () => client.deleteNode(node.id),
    onSuccess: async () => {
      setConfirm(null);
      toast.add({ title: t('node.danger.removed', { name: node.name }), type: 'success' });
      await navigate('/nodes', { replace: true });
      await cache.invalidateQueries({ queryKey: ['fleetStatus'] });
    },
  });
  const pending = reenroll.isPending || remove.isPending;
  const error = confirm === 'reenroll' ? reenroll.error : confirm === 'remove' ? remove.error : null;

  return (
    // One row: each action says what it does on hover, and its confirmation spells out the consequences.
    <section aria-labelledby='node-danger-title' className='flex flex-wrap items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-danger-border)] bg-[var(--color-surface-translucent)] p-4 sm:px-6'>
      <h2 className='me-auto text-sm font-semibold' id='node-danger-title'>{t('node.danger.title')}</h2>
      <Tooltip>
        <TooltipTrigger render={<Button onClick={() => setConfirm('reenroll')} variant='outline' />}>{t('node.danger.reenroll')}</TooltipTrigger>
        <TooltipContent>{t('node.danger.reenrollDescription')}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger render={<Button onClick={() => setConfirm('remove')} variant='destructive' />}>{t('node.danger.remove')}</TooltipTrigger>
        <TooltipContent>{t('node.danger.removeDescription')}</TooltipContent>
      </Tooltip>
      <AlertDialog
        onOpenChange={(open) => {
          if (!open && !pending) {
            setConfirm(null);
            reenroll.reset();
            remove.reset();
          }
        }}
        open={confirm !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirm === null ? '' : t(`node.danger.${confirm}Title`, { name: node.name })}</AlertDialogTitle>
            <AlertDialogDescription>{confirm === null ? '' : t(`node.danger.${confirm}Confirm`, { name: node.name })}</AlertDialogDescription>
          </AlertDialogHeader>
          {error ? <p className='text-sm text-[var(--color-danger-text)]' role='alert'>{describeRequestError(error)}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pending) return;
                if (confirm === 'reenroll') reenroll.mutate();
                else remove.mutate();
              }}
              variant={confirm === 'remove' ? 'destructive' : 'default'}
            >
              {confirm === 'remove'
                ? t(remove.isPending ? 'node.danger.removing' : 'node.danger.removeAction', { name: node.name })
                : t(reenroll.isPending ? 'node.danger.issuing' : 'node.danger.reenrollAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}

/** The node's name, published host and traffic period in one group, its version, and its re-enrollment and removal. */
export function NodeSettingsPage() {
  const { t } = useTranslation();
  const node = useCurrentNode();
  const client = useApiClient();
  const settings = useQuery(queries.nodeSettings(client));
  return (
    // The settings group takes the height above the destructive row, so its action bar sits at the view's foot.
    <section className='flex min-w-0 flex-1 flex-col gap-4'>
      <h1 className='sr-only'>{t('node.tabs.settings')}</h1>
      {settings.data
        ? <SettingsForm key={`${settings.data.revision}:${node.name}`} node={node} settings={settings.data} />
        : settings.isError
          ? (
              <div className='flex min-h-40 flex-col items-center justify-center gap-3 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] p-6 text-center' role='status'>
                <p className='text-sm'>{describeRequestError(settings.error)}</p>
                <Button onClick={() => void settings.refetch()} variant='outline'>{t('shell.error.retry')}</Button>
              </div>
            )
          : <LoadingState className='min-h-80' label={t('node.settings.loading')} fullScreen={false} />}
      <VersionRow node={node} />
      <DangerZone node={node} />
    </section>
  );
}
