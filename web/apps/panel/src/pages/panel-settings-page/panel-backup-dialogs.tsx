import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Check, Lock, Upload } from 'lucide-react';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { formatBytes } from '@shared/lib/telemetry-format';
import { useEffect, useId, useRef, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';
import { Dialog, DialogContent, DialogDescription, DialogFields, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { PanelBackupCategory, PanelBackupCounts, PanelBackupInfo, PanelRestorePlan, PanelRestoreStaged } from '@/api/api-client';

import { queries } from '@/api/queries';
import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { awaitRestart } from '@/components/status-line';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { HeightTransition } from '@/components/height-transition';
import { formatLogTimeCompact } from '@/utils/panel-log-presentation';
import { describeRequestError, toastFailure } from '@/components/error-notice';

import { BACKUP_CATEGORIES, categoryCount, categoryList, contentsLabel, DEFAULT_CATEGORIES, effectsLine } from './panel-backup';

const dataFace = 'font-[family-name:var(--font-data)] tabular-nums';
const sectionLabel = 'text-compact font-medium text-muted-foreground';
// On a phone the footer's note takes a line of its own above the buttons, kept even when empty.
const footerOnPhones = 'max-sm:[&>[role=status]]:basis-full';

function ordered(categories: ReadonlySet<PanelBackupCategory>) {
  return BACKUP_CATEGORIES.filter(category => categories.has(category));
}

function toggled(categories: ReadonlySet<PanelBackupCategory>, category: PanelBackupCategory) {
  const next = new Set(categories);
  if (!next.delete(category)) next.add(category);
  return next;
}

/**
 * The categories as checkable tiles, two to a row: each with its count when
 * one is known, and "—" when the backup does not hold it, which cannot be
 * chosen. Inert tiles show what will be offered without taking a choice.
 */
function CategoryTiles({ labelledBy, selected, onToggle, held, counts, inert }: {
  labelledBy: string;
  selected: ReadonlySet<PanelBackupCategory>;
  onToggle: (category: PanelBackupCategory) => void;
  /** The categories that can be chosen; every one when absent. */
  held?: readonly PanelBackupCategory[];
  counts?: PanelBackupCounts;
  inert?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  return (
    <div aria-labelledby={labelledBy} className={cn('grid grid-cols-2 gap-1.5 max-[520px]:grid-cols-1', inert && held?.length && 'opacity-60')} role='group'>
      {BACKUP_CATEGORIES.map((category) => {
        const available = held === undefined || held.includes(category);
        const checked = available && selected.has(category);
        const unavailable = !available || inert;
        let count: string | undefined;
        if (!available) count = '—';
        else if (counts) count = categoryCount(t, locale, category, counts);
        return (
          <button
            aria-checked={checked}
            aria-disabled={unavailable || undefined}
            className={cn(
              'flex h-10 min-w-0 items-center gap-2.5 rounded-[var(--radius-control)] border px-3 text-start text-sm outline-none transition-colors duration-150 ease-out focus-visible:border-primary',
              checked ? 'border-primary/50 bg-accent' : 'hover:bg-[var(--color-hover)]',
              unavailable && 'cursor-default hover:bg-transparent',
              !available && 'text-[var(--color-text-faint)]',
            )}
            key={category}
            onClick={() => {
              if (!unavailable) onToggle(category);
            }}
            role='checkbox'
            type='button'
          >
            <span
              aria-hidden='true'
              className={cn(
                'flex size-3.5 flex-none items-center justify-center rounded-[4px] border',
                checked ? 'border-primary bg-primary text-primary-foreground' : 'border-[var(--color-border-strong)]',
              )}
            >
              {checked && <Check className='size-3' strokeWidth={3} />}
            </span>
            <span className='min-w-0 truncate'>{t(`panelSettings.backupCategory.${category}`)}</span>
            {count ? <span className={`ms-auto shrink-0 text-compact text-[var(--color-text-faint)] ${dataFace}`}>{count}</span> : null}
          </button>
        );
      })}
    </div>
  );
}

/**
 * Backs the panel up now: the categories chosen, everything but the log at
 * first, and with a passphrase typed twice, encrypted. What stops it goes to
 * a toast, marking the field at fault.
 */
export function CreateBackupDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const contentsId = useId();
  const [categories, setCategories] = useState<ReadonlySet<PanelBackupCategory>>(() => new Set(DEFAULT_CATEGORIES));
  const [passphrase, setPassphrase] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [invalid, setInvalid] = useState<'passphrase' | 'confirmation' | null>(null);
  const [pending, setPending] = useState(false);
  const passphraseRef = useRef<HTMLInputElement>(null);
  const confirmationRef = useRef<HTMLInputElement>(null);

  function reset() {
    setCategories(new Set(DEFAULT_CATEGORIES));
    setPassphrase('');
    setConfirmation('');
    setInvalid(null);
  }

  async function create() {
    if (pending) return;
    if (categories.size === 0) {
      toast.add({ title: t('panelSettings.backupCategoriesNone'), type: 'error' });
      return;
    }
    if (passphrase !== '' && [...passphrase].length < 8) {
      setInvalid('passphrase');
      passphraseRef.current?.focus();
      toast.add({ title: t('panelSettings.backupPassphraseShort'), type: 'error' });
      return;
    }
    if (passphrase !== confirmation) {
      setInvalid('confirmation');
      confirmationRef.current?.focus();
      toast.add({ title: t('panelSettings.backupPassphraseMismatch'), type: 'error' });
      return;
    }
    setPending(true);
    try {
      await client.createPanelBackup({ categories: ordered(categories), ...(passphrase !== '' && { passphrase }) });
      onOpenChange(false);
      reset();
      void cache.invalidateQueries({ queryKey: ['panelBackups'] });
      toast.add({ title: t('panelSettings.backupCreated'), type: 'success' });
    } catch (cause) {
      toastFailure(t('panelSettings.backupFailed'), cause);
    } finally {
      setPending(false);
    }
  }

  const partial = categories.size > 0 && !DEFAULT_CATEGORIES.every(category => categories.has(category));
  const list = categoryList(t, ordered(categories));
  const message = partial ? <span className='text-[var(--color-text-faint)]'>{t('panelSettings.backupPartial', { list })}</span> : '';

  return (
    <Dialog open={open} onOpenChange={(next) => {
      if (pending) return;
      onOpenChange(next);
      if (!next) reset();
    }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('panelSettings.backupCreateTitle')}</DialogTitle>
          <DialogDescription>{t('panelSettings.backupCreateDescription')}</DialogDescription>
        </DialogHeader>
        <form className='contents' noValidate onSubmit={(event) => {
          event.preventDefault();
          // React carries a submit out of the portal to the settings form, which would save the draft.
          event.stopPropagation();
          void create();
        }}>
          <section className='grid gap-2'>
            <h3 className={sectionLabel} id={contentsId}>{t('panelSettings.backupContents')}</h3>
            <CategoryTiles
              labelledBy={contentsId}
              onToggle={(category) => {
                setCategories(current => toggled(current, category));
              }}
              selected={categories}
            />
          </section>
          <section className='grid gap-2'>
            <label className={sectionLabel} htmlFor='backup-passphrase'>{t('panelSettings.backupPassphrase')}</label>
            <div className='grid grid-cols-2 gap-2 max-[520px]:grid-cols-1'>
              <Input
                aria-invalid={invalid === 'passphrase' || undefined}
                autoComplete='new-password'
                id='backup-passphrase'
                maxLength={1024}
                onChange={(event) => {
                  setPassphrase(event.target.value);
                  if (event.target.value === '') setConfirmation('');
                  setInvalid(null);
                }}
                placeholder={t('panelSettings.backupPassphrasePlaceholder')}
                ref={passphraseRef}
                type='password'
                value={passphrase}
              />
              {/* Always there, hidden until a passphrase is typed, so the dialog keeps its size. */}
              <Input
                aria-invalid={invalid === 'confirmation' || undefined}
                aria-label={t('panelSettings.backupPassphraseConfirm')}
                autoComplete='new-password'
                className={passphrase === '' ? 'invisible' : undefined}
                maxLength={1024}
                onChange={(event) => {
                  setConfirmation(event.target.value);
                  setInvalid(null);
                }}
                placeholder={t('panelSettings.backupPassphraseConfirm')}
                ref={confirmationRef}
                type='password'
                value={confirmation}
              />
            </div>
          </section>
          <DialogFooter className={footerOnPhones} message={message}>
            {/* Closing waits for the backup, so a failure leaves the choices to try again. */}
            <Button onClick={() => !pending && onOpenChange(false)} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button type='submit'>
              {pending && <Spinner aria-hidden='true' data-icon='inline-start' />}
              {t(pending ? 'panelSettings.backupCreating' : 'panelSettings.backupCreate')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Where a restore comes from: a backup in the panel's folder, or a file the browser holds. */
type Source = { kind: 'stored'; backup: PanelBackupInfo } | { kind: 'file'; file: File };

type CheckState
  = | { step: 'idle' }
    | { step: 'locked'; incorrect?: boolean }
    | { step: 'checking'; sent?: number; total?: number }
    | { step: 'failed' }
    | { step: 'ready'; staged: PanelRestoreStaged };

/**
 * Restores a backup. A new dialog starts on each opening, with the backup it
 * was opened for chosen, if any.
 */
interface RestoreDialogProps {
  open: boolean;
  onClose: () => void;
  /** The backup chosen when the dialog opens, if any. */
  initial: PanelBackupInfo | null;
}

export function RestoreBackupDialog({ open, initial, onClose }: RestoreDialogProps) {
  const [opening, setOpening] = useState(0);
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setOpening(count => count + 1);
  }
  return <RestoreDialog initial={initial} key={opening} onClose={onClose} open={open} />;
}

/**
 * The backups to pick from, the panel's own and a file from this computer,
 * then what to restore of the one picked. The panel verifies a backup whole
 * once it is picked, an encrypted one once its passphrase is given; the
 * categories it holds are then offered, each with its count, and the footer
 * says what restoring only some changes in the rest. Restoring backs up the
 * current state and restarts the panel.
 */
function RestoreDialog({ open, initial, onClose }: RestoreDialogProps) {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const cache = useQueryClient();
  const { view } = usePanelSettings();
  const backups = useQuery(queries.panelBackups(client));
  const pickId = useId();
  const contentsId = useId();
  const fileRef = useRef<HTMLInputElement>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const [source, setSource] = useState<Source | null>(initial ? { kind: 'stored', backup: initial } : null);
  const [check, setCheck] = useState<CheckState>(() => {
    if (!initial) return { step: 'idle' };
    return initial.encrypted ? { step: 'locked' } : { step: 'checking' };
  });
  const [passphrase, setPassphrase] = useState('');
  const [selected, setSelected] = useState<ReadonlySet<PanelBackupCategory>>(() => new Set(initial?.categories ?? []));
  const [planned, setPlanned] = useState<{ key: string; plan: PanelRestorePlan } | null>(null);
  const [takeService, setTakeService] = useState(false);
  const [applying, setApplying] = useState(false);

  async function stage(from: Source, secret: string) {
    controllerRef.current?.abort();
    const abort = new AbortController();
    controllerRef.current = abort;
    setCheck({ step: 'checking' });
    try {
      const staged = from.kind === 'file'
        ? await client.uploadPanelRestore(from.file, secret, (sent, total) => setCheck({ step: 'checking', sent, total }), abort.signal)
        : await client.stagePanelBackup(from.backup.name, secret === '' ? {} : { passphrase: secret }, abort.signal);
      if (abort.signal.aborted) return;
      setSelected(new Set(staged.manifest.categories));
      setCheck({ step: 'ready', staged });
    } catch (cause) {
      if (abort.signal.aborted) return;
      const code = cause instanceof ApiRequestError ? cause.code : '';
      if (code === 'backup_passphrase_required' || code === 'backup_passphrase_incorrect') {
        setCheck({ step: 'locked', incorrect: secret !== '' });
        if (secret !== '') toast.add({ title: describeRequestError(cause), type: 'error' });
      } else {
        setCheck({ step: 'failed' });
        toastFailure(t('panelSettings.backupCheckFailed'), cause);
      }
    }
  }

  useEffect(() => {
    if (initial && !initial.encrypted) void stage({ kind: 'stored', backup: initial }, '');
    return () => controllerRef.current?.abort();
    // Only the backup the dialog opened for is checked at once.
    // eslint-disable-next-line react/exhaustive-deps
  }, []);

  function discardStaged() {
    if (check.step === 'ready') void client.discardPanelRestore(check.staged.id).catch(() => undefined);
  }

  function choose(next: Source) {
    controllerRef.current?.abort();
    discardStaged();
    setSource(next);
    setPassphrase('');
    setTakeService(false);
    setSelected(new Set(next.kind === 'stored' ? next.backup.categories : []));
    if (next.kind === 'stored' && next.backup.encrypted) setCheck({ step: 'locked' });
    else void stage(next, '');
  }

  function close() {
    controllerRef.current?.abort();
    discardStaged();
    onClose();
  }

  const staged = check.step === 'ready' ? check.staged : null;
  const planKey = staged ? `${staged.id}:${ordered(selected).join(',')}` : '';
  useEffect(() => {
    if (!staged || selected.size === 0) return;
    const abort = new AbortController();
    client.planPanelRestore(staged.id, { categories: ordered(selected) }, abort.signal)
      .then(plan => setPlanned({ key: planKey, plan }))
      .catch(() => undefined);
    return () => abort.abort();
    // planKey names the staged restore and the choice.
    // eslint-disable-next-line react/exhaustive-deps
  }, [planKey]);
  const plan = planned?.key === planKey ? planned.plan : null;

  async function apply() {
    if (applying) return;
    if (!staged) {
      toast.add({ title: t(source ? 'panelSettings.backupPassphraseRequired' : 'panelSettings.backupPickFirst'), type: 'error' });
      return;
    }
    if (selected.size === 0) {
      toast.add({ title: t('panelSettings.backupCategoriesNone'), type: 'error' });
      return;
    }
    setApplying(true);
    try {
      const requested = await client.applyPanelRestore(staged.id, {
        take_service: takeService, categories: ordered(selected),
      });
      onClose();
      if (!requested.restarting) {
        // The backup of the current state is new in the list.
        void cache.invalidateQueries({ queryKey: ['panelBackups'] });
        toast.add({ title: t('panelSettings.backupRestoreNextStart'), type: 'info' });
        return;
      }
      const outcome = await awaitRestart({ cache, client, t }, async () => requested.started_at);
      if (outcome.kind === 'restarted') toast.add({ title: t('panelSettings.backupRestored'), type: 'success' });
      else if (outcome.kind === 'failed') toast.add({ title: outcome.reason, type: 'error' });
    } catch (cause) {
      setApplying(false);
      toastFailure(t('panelSettings.backupRestoreFailed'), cause);
    }
  }

  const here = view
    ? { host: view.preferences.listen_host, port: view.preferences.listen_port, base_path: view.service.base_path }
    : null;
  const address = (server: { host: string; port: number; base_path: string }) => `${server.host}:${server.port}${server.base_path}`;
  const listenerDiffers = Boolean(staged?.server && here && selected.has('settings') && address(here) !== address(staged.server));
  const held = staged?.manifest.categories ?? (source?.kind === 'stored' ? source.backup.categories : []);
  const counts = staged?.manifest.snapshot.counts ?? (source?.kind === 'stored' ? source.backup.counts : undefined);
  const signsOut = plan?.signs_out ?? selected.has('settings');

  // The footer's line carries progress and what the choice changes; failures go to toasts.
  let message: ReactNode = '';
  if (check.step === 'checking') {
    message = (
      <span className='inline-flex items-center gap-2 text-muted-foreground'>
        <Spinner aria-hidden='true' className='size-3.5' />
        {t(check.total ? 'panelSettings.backupUploading' : 'panelSettings.backupChecking')}
        {check.total ? <span className={dataFace}>{`${formatBytes(check.sent ?? 0, locale)} / ${formatBytes(check.total, locale)}`}</span> : null}
      </span>
    );
  } else if (staged) {
    // Notes share the footer's one line, so choosing never moves the dialog.
    const notes = [
      signsOut ? t('panelSettings.backupSignsOut') : '',
      staged.manifest.snapshot.counts.nodes > 0 && selected.has('nodes') && !staged.same_panel ? t('panelSettings.backupOtherPanel') : '',
      plan ? effectsLine(t, plan.effects) : '',
    ].filter(Boolean);
    message = notes.length > 0 ? <span className='text-[var(--color-warning-text)]'>{notes.join(t('panelSettings.backupEffectSeparator'))}</span> : '';
  }

  const rows: { key: string; source: Source; time: string; detail: string; encrypted: boolean; size: number }[] = [];
  if (source?.kind === 'file') {
    rows.push({ key: 'file', source, time: '', detail: source.file.name, encrypted: false, size: source.file.size });
  }
  for (const backup of backups.data?.items ?? []) {
    const contents = contentsLabel(t, backup.categories);
    rows.push({
      key: backup.name, source: { kind: 'stored', backup }, encrypted: backup.encrypted, size: backup.size,
      time: formatLogTimeCompact(backup.created_at, locale, { seconds: false }),
      detail: `${t(`panelSettings.backupKind.${backup.kind}`)}${contents ? ` · ${contents}` : ''}`,
    });
  }
  const isChosen = (row: (typeof rows)[number]) => source !== null && (row.source.kind === 'file'
    ? source.kind === 'file'
    : source.kind === 'stored' && source.backup.name === row.source.backup.name);
  const focusable = rows.findIndex(isChosen) === -1 ? 0 : rows.findIndex(isChosen);

  return (
    <Dialog open={open} onOpenChange={(next) => {
      if (!next && !applying) close();
    }}>
      {/* Its top stays where its tallest form centres it, so a passphrase or listener row grows it downward. */}
      <DialogContent className='top-[max(1rem,calc(50dvh-19rem))] translate-y-0' size='wide'>
        <DialogHeader>
          <DialogTitle>{t('panelSettings.backupRestoreTitle')}</DialogTitle>
          <DialogDescription>{t('panelSettings.backupRestoreDescription')}</DialogDescription>
        </DialogHeader>
        <HeightTransition className='grid gap-5'>
          <section className='grid gap-2'>
            <h3 className={sectionLabel} id={pickId}>{t('panelSettings.backupPick')}</h3>
            <div className='overflow-hidden rounded-[var(--radius-control)] border'>
              <ScrollArea className='max-h-[calc(4*2.25rem+3px)]'>
                <div aria-labelledby={pickId} className='divide-y' role='radiogroup'>
                  {rows.length === 0 && (
                    <p className='flex h-9 items-center justify-center text-compact text-muted-foreground' role='status'>
                      {t(backups.isPending ? 'panelSettings.backupLoading' : 'panelSettings.backupNone')}
                    </p>
                  )}
                  {rows.map((row, index) => {
                    const chosen = isChosen(row);
                    return (
                      <button
                        aria-checked={chosen}
                        aria-label={[row.time, row.detail, row.encrypted ? t('panelSettings.backupEncrypted') : ''].filter(Boolean).join(' · ')}
                        className={cn(
                          'grid h-9 w-full grid-cols-[0.75rem_auto_minmax(0,1fr)_1rem_4rem] items-center gap-2.5 px-3 text-start text-compact outline-none',
                          'focus-visible:bg-[var(--color-hover)] max-sm:grid-cols-[0.75rem_auto_minmax(0,1fr)_1rem]',
                          chosen ? 'bg-accent' : 'hover:bg-[var(--color-hover)]',
                        )}
                        key={row.key}
                        onClick={() => {
                          if (!chosen && !applying) choose(row.source);
                        }}
                        onKeyDown={(event) => {
                          const step = { ArrowDown: 1, ArrowUp: -1 }[event.key];
                          if (!step) return;
                          event.preventDefault();
                          const target = (index + step + rows.length) % rows.length;
                          const group = event.currentTarget.parentElement;
                          if (!applying) choose(rows[target].source);
                          (group?.children[target] as HTMLElement | undefined)?.focus();
                        }}
                        role='radio'
                        tabIndex={index === focusable ? 0 : -1}
                        type='button'
                      >
                        <span aria-hidden='true' className={cn('size-3 rounded-full border', chosen ? 'border-[4px] border-primary' : 'border-[var(--color-border-strong)]')} />
                        {row.source.kind === 'file'
                          ? <Upload aria-hidden='true' className='size-3.5 text-[var(--color-text-faint)]' />
                          : <span className={dataFace}>{row.time}</span>}
                        <span className='truncate text-[var(--color-text-faint)]' title={row.detail}>{row.detail}</span>
                        <span className='flex justify-center text-[var(--color-text-faint)]'>
                          {row.encrypted && <Lock aria-label={t('panelSettings.backupEncrypted')} className='size-3.5' role='img' />}
                        </span>
                        <span className={`text-end text-[var(--color-text-secondary)] max-sm:hidden ${dataFace}`}>{formatBytes(row.size, locale)}</span>
                      </button>
                    );
                  })}
                </div>
              </ScrollArea>
              <button
                className='flex h-9 w-full items-center gap-2 border-t border-dashed px-3 text-compact text-[var(--color-text-secondary)] outline-none hover:bg-[var(--color-hover)] focus-visible:bg-[var(--color-hover)]'
                onClick={() => fileRef.current?.click()}
                type='button'
              >
                <Upload aria-hidden='true' className='size-3.5' />
                {t('panelSettings.backupChooseFile')}
              </button>
              <input
                accept='.gz,application/gzip,application/octet-stream'
                aria-label={t('panelSettings.backupChooseFile')}
                className='sr-only'
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  if (file) choose({ kind: 'file', file });
                  event.target.value = '';
                }}
                ref={fileRef}
                tabIndex={-1}
                type='file'
              />
            </div>
            {check.step === 'locked' && source && (
              <form className='flex items-center gap-2' noValidate onSubmit={(event) => {
                event.preventDefault();
                event.stopPropagation();
                void stage(source, passphrase);
              }}>
                <Input
                  aria-invalid={check.incorrect || undefined}
                  aria-label={t('panelSettings.backupPassphrase')}
                  autoComplete='off'
                  autoFocus
                  className='min-w-0 flex-1'
                  maxLength={1024}
                  onChange={(event) => {
                    setPassphrase(event.target.value);
                    setCheck({ step: 'locked' });
                  }}
                  placeholder={t('panelSettings.backupPassphrase')}
                  type='password'
                  value={passphrase}
                />
                <Button type='submit' variant='outline'>{t('panelSettings.backupUnlock')}</Button>
              </form>
            )}
          </section>
          <section className='grid gap-2'>
            <h3 className={sectionLabel} id={contentsId}>{t('panelSettings.backupRestoreContents')}</h3>
            <CategoryTiles
              counts={counts}
              held={held}
              inert={!staged}
              labelledBy={contentsId}
              onToggle={(category) => {
                setSelected(current => toggled(current, category));
              }}
              selected={selected}
            />
            {listenerDiffers && staged?.server && here && (
              <DialogFields>
                <label htmlFor='restore-listener'>{t('panelSettings.backupListener')}</label>
                <Select<string> onValueChange={value => setTakeService(value === 'take')} value={takeService ? 'take' : 'keep'}
                  items={[
                    { value: 'keep', label: `${t('panelSettings.backupKeepListener')} · ${address(here)}` },
                    { value: 'take', label: `${t('panelSettings.backupTakeListener')} · ${address(staged.server)}` },
                  ]}
                >
                  <SelectTrigger id='restore-listener'><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value='keep'>{`${t('panelSettings.backupKeepListener')} · ${address(here)}`}</SelectItem>
                      <SelectItem value='take'>{`${t('panelSettings.backupTakeListener')} · ${address(staged.server)}`}</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </DialogFields>
            )}
          </section>
        </HeightTransition>
        <DialogFooter className={footerOnPhones} message={message}>
          <Button onClick={() => !applying && close()} type='button' variant='outline'>{t('common.cancel')}</Button>
          <Button onClick={() => void apply()} type='button' variant='destructive'>
            {applying && <Spinner aria-hidden='true' data-icon='inline-start' />}
            {t(applying ? 'panelSettings.backupApplying' : 'panelSettings.backupApply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
