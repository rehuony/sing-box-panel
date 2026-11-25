import type { FormEvent } from 'react';

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { formatDateTime } from '@shared/lib/format-time';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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
import { messageLine } from '@/utils/message-line';
import { useApiClient } from '@/api/api-client-context';
import { RegionError } from '@/components/region-state';
import { LoadingState } from '@/components/loading-state';
import { MAX_TAGS, TagInput } from '@/components/tag-input';
import { useCurrentNode } from '@/stores/current-node.store';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { useEnrollmentDialog } from '@/stores/enrollment-dialog.store';
import { SettingsCard, SettingsCardState, SettingsField } from '@/components/settings-field';

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

  const changed = (field: DraftField) => field === 'tags' ? draft.tags.join() !== saved.tags.join() : draft[field] !== saved[field];
  const row = (field: DraftField) => ({ changed: changed(field), invalid: invalid === field });
  const unsaved = (['name', 'publicHost', 'tags', 'quota', 'months', 'resetDay'] as const).filter(changed).length;
  // The bar's one line: why saving failed or what it refused, otherwise how many rows wait to be saved.
  const line = message ? messageLine(message) : unsaved > 0 ? t('node.settings.unsaved', { count: unsaved }) : '';
  return (
    <form className='min-w-0' noValidate onSubmit={submit}>
      <SettingsCard
        footer={(
          <>
            <p className={message && invalid !== null ? 'min-w-0 flex-1 truncate text-sm text-[var(--color-danger-text)]' : 'min-w-0 flex-1 truncate text-sm text-muted-foreground'} role='status' title={message ? line : undefined}>{line}</p>
            <Button onClick={() => {
              setDraft(saved);
              setInvalid(null);
              setMessage('');
            }} type='button' variant='outline'>
              {t('node.settings.reset')}
            </Button>
            <Button type='submit'>{t(save.isPending ? 'node.settings.saving' : 'node.settings.save')}</Button>
          </>
        )}
        title={t('node.settings.title')}
      >
        <SettingsField {...row('name')} id='node-name' label={t('node.settings.name')}>
          <Input aria-invalid={invalid === 'name' || undefined} autoComplete='off' id='node-name' maxLength={64} onChange={event => update('name', event.target.value)} spellCheck={false} value={draft.name} />
        </SettingsField>
        <SettingsField {...row('tags')} id='node-tags' label={t('node.settings.tags')}>
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
        <SettingsField {...row('publicHost')} help={t('node.settings.publicHostHelp')} id='node-public-host' label={t('node.settings.publicHost')}>
          <Input aria-invalid={invalid === 'publicHost' || undefined} autoComplete='off' className='font-mono' id='node-public-host' maxLength={253} onChange={event => update('publicHost', event.target.value)} placeholder={node.public_ip || 'node.example.com'} spellCheck={false} value={draft.publicHost} />
        </SettingsField>
        <SettingsField {...row('quota')} help={t('node.settings.quotaHelp')} id='node-quota' label={t('node.settings.quota')}>
          <InputGroup>
            <InputGroupInput aria-invalid={invalid === 'quota' || undefined} className='tabular-nums' id='node-quota' inputMode='numeric' min={0} onChange={event => update('quota', event.target.value)} placeholder={t('node.settings.unlimited')} type='number' value={draft.quota} />
            <InputGroupAddon align='inline-end'>GiB</InputGroupAddon>
          </InputGroup>
        </SettingsField>
        <SettingsField {...row('months')} id='node-period' label={t('node.settings.period')}>
          <InputGroup>
            <InputGroupInput aria-invalid={invalid === 'months' || undefined} className='tabular-nums' id='node-period' inputMode='numeric' max={120} min={1} onChange={event => update('months', event.target.value)} type='number' value={draft.months} />
            <InputGroupAddon align='inline-end'>{t('node.settings.months')}</InputGroupAddon>
          </InputGroup>
        </SettingsField>
        <SettingsField {...row('resetDay')} help={t('node.settings.resetDayHelp')} id='node-reset-day' label={t('node.settings.resetDay')}>
          <InputGroup>
            <InputGroupInput aria-invalid={invalid === 'resetDay' || undefined} className='tabular-nums' id='node-reset-day' inputMode='numeric' max={28} min={1} onChange={event => update('resetDay', event.target.value)} type='number' value={draft.resetDay} />
            {/* Languages that name the day of the month with a unit show it; English needs none. */}
            {t('node.settings.day') ? <InputGroupAddon align='inline-end'>{t('node.settings.day')}</InputGroupAddon> : null}
          </InputGroup>
        </SettingsField>
      </SettingsCard>
    </form>
  );
}

/**
 * What the node reported about itself, each with the action that changes it
 * beside it: its host, system and public address, its version against the
 * panel's release with the update, and its enrollment with re-enrollment.
 */
function ProfileCard({ node }: { node: FleetNode }) {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const client = useApiClient();
  const cache = useQueryClient();
  const [confirm, setConfirm] = useState<'update' | 'reenroll' | null>(null);
  const update = useMutation({
    mutationFn: () => client.updateNode(),
    onSuccess: (result) => {
      setConfirm(null);
      toast.add({ title: t(`node.version.result.${result.state}`, { name: node.name, version: result.version }), type: 'success' });
    },
    onSettled: () => cache.invalidateQueries({ queryKey: ['fleetStatus'] }),
  });
  const reenroll = useMutation({
    mutationFn: () => client.createNodeEnrollment(node.id),
    onSuccess: (enrollment) => {
      setConfirm(null);
      useEnrollmentDialog.getState().reenroll({ enrollment, enrolledBefore: node.enrolled_at });
    },
  });
  const pending = update.isPending || reenroll.isPending;
  const error = confirm === 'update' ? update.error : confirm === 'reenroll' ? reenroll.error : null;
  const last = node.last_update;
  const versionDetail = last && !last.succeeded
    ? t('node.version.lastFailed', { version: last.version, code: last.code })
    : node.update_available ? t('node.version.available') : t('node.version.current');
  const value = 'font-[family-name:var(--font-data)] text-compact';
  // A value's detail sits on a line of its own below it; each line keeps to one line and shows all on hover.
  const stacked = 'flex min-w-0 flex-1 flex-col *:truncate';
  const meta = 'text-xs text-muted-foreground';
  return (
    <SettingsCard title={t('node.profile.title')}>
      <SettingsField id='node-hostname' label={t('node.profile.hostname')}>
        <output className={`flex min-h-10 min-w-0 items-center ${value}`} id='node-hostname'><span className='truncate'>{node.hostname || '—'}</span></output>
      </SettingsField>
      <SettingsField id='node-system' label={t('node.profile.system')}>
        <output className={`flex min-h-10 items-center ${value}`} id='node-system'>{node.os && node.arch ? `${node.os}/${node.arch}` : '—'}</output>
      </SettingsField>
      <SettingsField id='node-public-ip' label={t('node.profile.publicIp')}>
        <output className={`flex min-h-10 items-center ${value}`} id='node-public-ip'>{node.public_ip || '—'}</output>
      </SettingsField>
      <SettingsField id='node-version' label={t('node.version.title')}>
        <div className='flex min-h-10 items-center gap-3'>
          <output className={stacked} id='node-version'>
            <span className={value} title={node.build_version || undefined}>{node.build_version || '—'}</span>
            <span className={meta} title={versionDetail}>{versionDetail}</span>
          </output>
          {node.update_available && (
            <Button onClick={() => {
              update.reset();
              setConfirm('update');
            }} size='sm' type='button' variant='outline'>
              {t('node.version.update')}
            </Button>
          )}
        </div>
      </SettingsField>
      <SettingsField help={t('node.danger.reenrollDescription')} id='node-enrolled' label={t('node.profile.enrolled')}>
        <div className='flex min-h-10 items-center gap-3'>
          <output className={stacked} id='node-enrolled'>
            <time className={value} dateTime={node.enrolled_at}>{formatDateTime(node.enrolled_at, locale) || '—'}</time>
            {node.created_at && node.created_at !== node.enrolled_at && (
              <span className={meta}>{t('node.profile.firstEnrolled', { time: formatDateTime(node.created_at, locale) })}</span>
            )}
          </output>
          <Button onClick={() => {
            reenroll.reset();
            setConfirm('reenroll');
          }} size='sm' type='button' variant='outline'>
            {t('node.danger.reenroll')}
          </Button>
        </div>
      </SettingsField>
      <AlertDialog onOpenChange={(open) => {
        if (!open && !pending) setConfirm(null);
      }} open={confirm !== null}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirm === 'update' ? t('node.version.confirmTitle', { name: node.name }) : t('node.danger.reenrollTitle', { name: node.name })}</AlertDialogTitle>
            <AlertDialogDescription>
              {confirm === 'update'
                ? t('node.version.confirm', { name: node.name, current: node.build_version })
                : t('node.danger.reenrollConfirm', { name: node.name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {error ? <p className='text-sm text-[var(--color-danger-text)]' role='alert'>{describeRequestError(error)}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => {
              if (pending) return;
              if (confirm === 'update') update.mutate();
              else reenroll.mutate();
            }}>
              {confirm === 'update'
                ? t(update.isPending ? 'node.version.updating' : 'node.version.updateAction')
                : t(reenroll.isPending ? 'node.danger.issuing' : 'node.danger.reenrollAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsCard>
  );
}

/** The node's removal from the panel, apart from everything else and confirmed in danger. */
function RemoveCard({ node }: { node: FleetNode }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const navigate = useNavigate();
  const [confirming, setConfirming] = useState(false);
  const remove = useMutation({
    mutationFn: () => client.deleteNode(node.id),
    onSuccess: async () => {
      setConfirming(false);
      toast.add({ title: t('node.danger.removed', { name: node.name }), type: 'success' });
      await navigate('/nodes', { replace: true });
      await cache.invalidateQueries({ queryKey: ['fleetStatus'] });
    },
  });
  return (
    <SettingsCard danger title={t('node.danger.title')}>
      <SettingsField help={t('node.danger.removeDescription')} id='node-remove' label={t('node.danger.remove')}>
        <div className='flex min-h-10 items-center justify-end' id='node-remove'>
          <Button onClick={() => {
            remove.reset();
            setConfirming(true);
          }} size='sm' type='button' variant='destructive'>
            {t('node.danger.remove')}
          </Button>
        </div>
      </SettingsField>
      <AlertDialog onOpenChange={(open) => {
        if (!open && !remove.isPending) setConfirming(false);
      }} open={confirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('node.danger.removeTitle', { name: node.name })}</AlertDialogTitle>
            <AlertDialogDescription>{t('node.danger.removeConfirm', { name: node.name })}</AlertDialogDescription>
          </AlertDialogHeader>
          {remove.error ? <p className='text-sm text-[var(--color-danger-text)]' role='alert'>{describeRequestError(remove.error)}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => {
              if (!remove.isPending) remove.mutate();
            }} variant='destructive'>
              {t(remove.isPending ? 'node.danger.removing' : 'node.danger.removeAction', { name: node.name })}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsCard>
  );
}

/** The node's settings beside its profile, which carries its update and re-enrollment, and its removal apart. */
export function NodeSettingsPage() {
  const { t } = useTranslation();
  const node = useCurrentNode();
  const client = useApiClient();
  const settings = useQuery(queries.nodeSettings(client));
  return (
    <section className='grid min-w-0 flex-none items-start gap-3 @5xl:grid-cols-2'>
      <h1 className='sr-only'>{t('node.tabs.settings')}</h1>
      <div className='flex min-w-0 flex-col gap-3'>
        {settings.data
          ? <SettingsForm key={`${settings.data.revision}:${node.name}`} node={node} settings={settings.data} />
          : (
              // As tall as the form's six rows and action bar, so nothing moves when they arrive.
              <SettingsCardState footer rows={6} title={t('node.settings.title')}>
                {settings.isError
                  ? <RegionError className='min-h-0 flex-1' error={settings.error} reload={() => void settings.refetch()} title={t('node.settings.unavailable')} />
                  : <LoadingState className='min-h-0 flex-1' label={t('node.settings.loading')} fullScreen={false} />}
              </SettingsCardState>
            )}
      </div>
      <div className='flex min-w-0 flex-col gap-3'>
        <ProfileCard node={node} />
        <RemoveCard node={node} />
      </div>
    </section>
  );
}
