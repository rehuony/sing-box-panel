import { useTranslation } from 'react-i18next';
import { useEffect, useId, useRef, useState } from 'react';
import { ArrowRight, Check, ChevronRight, Eye, EyeOff, File, Folder, FolderUp, Link, Plug, RefreshCw, Search, SquarePen, X } from 'lucide-react';

import type { FilesystemEntry, FilesystemMode } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { ApiRequestError } from '@/api/api-client';
import { ListPagination } from '@/components/list-pagination';
import { DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Field, FieldError, FieldLabel } from '@/components/ui/field';
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';

import { useServerPathPicker } from './use-server-path-picker';

export function ServerPathPicker({ initialValue, mode, open, onSelect }: {
  initialValue: string;
  mode: FilesystemMode;
  open: boolean;
  onSelect: (path: string) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const picker = useServerPathPicker(initialValue, mode, open, onSelect);
  const [editingLocation, setEditingLocation] = useState(false);
  const [locationDraft, setLocationDraft] = useState('');
  const locationInputRef = useRef<HTMLInputElement>(null);
  const editButtonRef = useRef<HTMLButtonElement>(null);
  const breadcrumbRef = useRef<HTMLElement>(null);
  const entriesContainerRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const focusEntriesRef = useRef(false);
  const { data, pending, checking, error, query } = picker;
  const blocked = pending || checking;
  const currentPath = (pending ? query.path : data?.path) ?? query.path ?? initialValue;
  const pathWithoutTrailingSlash = currentPath.replace(/\/+$/, '');
  const parentPath = data?.path === currentPath
    ? data.parent
    : currentPath.startsWith('/') ? pathWithoutTrailingSlash.slice(0, pathWithoutTrailingSlash.lastIndexOf('/')) || '/' : undefined;
  const parts = currentPath.split('/').filter(Boolean);
  const breadcrumbs = [{ name: '/', path: '/' }, ...parts.map((name, index) => ({ name, path: `/${parts.slice(0, index + 1).join('/')}` }))];
  const errorCode = error instanceof ApiRequestError ? error.code : 'filesystem_failed';
  const errorKey = `filesystem.errors.${errorCode}`;
  const errorMessage = t(errorKey, { defaultValue: t('filesystem.errors.filesystem_failed') });

  useEffect(() => {
    if (editingLocation) {
      locationInputRef.current?.focus();
      locationInputRef.current?.select();
    }
  }, [editingLocation]);

  useEffect(() => {
    const breadcrumb = breadcrumbRef.current;
    if (!breadcrumb) return;
    const revealCurrentDirectory = () => {
      breadcrumb.scrollLeft = breadcrumb.scrollWidth;
    };
    revealCurrentDirectory();
    const observer = new ResizeObserver(revealCurrentDirectory);
    observer.observe(breadcrumb);
    return () => observer.disconnect();
  }, [currentPath, editingLocation, checking]);

  useEffect(() => {
    if (!open || pending || !focusEntriesRef.current) return;
    focusEntriesRef.current = false;
    const first = listRef.current?.querySelector<HTMLButtonElement>('button[data-path-entry]:not(:disabled)');
    (first ?? editButtonRef.current)?.focus();
  }, [open, pending, data]);

  function enterDirectory(path: string) {
    focusEntriesRef.current = true;
    // Keep focus inside a mounted node while rows are replaced by the loader.
    // Otherwise the dialog's focus recovery can override the new row's focus.
    entriesContainerRef.current?.focus({ preventScroll: true });
    picker.navigate(path);
  }

  function finishEditing(navigate: boolean) {
    if (navigate) picker.navigate(locationDraft);
    setEditingLocation(false);
    editButtonRef.current?.focus();
  }

  function canSelect(entry: FilesystemEntry) {
    return entry.available && (entry.kind === 'directory' || entry.kind === (mode === 'output-file' ? 'file' : mode));
  }

  function confirm() {
    if (blocked || editingLocation) return;
    void picker.confirm(picker.selectedPath);
  }

  return (
    <>
      <DialogHeader className='gap-1.5'>
        <DialogTitle>{t(`filesystem.titles.${mode}`)}</DialogTitle>
      </DialogHeader>
      <div className='flex min-h-0 min-w-0 flex-col gap-4 overflow-y-auto'>
        <div className='grid min-w-0 gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
          <div className='flex h-(--control-height) min-w-0 items-center gap-1'>
            {editingLocation
              ? (
                  <InputGroup className='min-w-0 flex-1'>
                    <InputGroupInput ref={locationInputRef} aria-label={t('filesystem.location')}
                      value={locationDraft}
                      disabled={checking} autoComplete='off' spellCheck={false}
                      onChange={event => setLocationDraft(event.target.value)} onKeyDown={event => {
                        if (event.key === 'Enter' || event.key === 'Escape') {
                          event.preventDefault();
                          event.stopPropagation();
                          finishEditing(event.key === 'Enter');
                        }
                      }} />
                    <InputGroupAddon align='inline-end'>
                      <InputGroupButton size='icon-sm' className='size-8 min-h-8' disabled={checking} aria-label={t('filesystem.go')} title={t('filesystem.go')}
                        onClick={() => finishEditing(true)}>
                        <ArrowRight className='size-4' strokeWidth={1.75} aria-hidden='true' />
                      </InputGroupButton>
                    </InputGroupAddon>
                  </InputGroup>
                )
              : (
                  <nav ref={breadcrumbRef} aria-label={t('filesystem.breadcrumb')} title={currentPath}
                    className='min-w-0 flex-1 self-center overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden'>
                    <ol className='flex w-max items-center gap-0.5'>
                      {breadcrumbs.map((crumb, index) => (
                        <li key={crumb.path} className='flex shrink-0 items-center'>
                          {index > 0 && <ChevronRight className='size-3.5 shrink-0 text-muted-foreground/50' aria-hidden='true' />}
                          {index === breadcrumbs.length - 1
                            ? <span aria-current='location' className='px-2 py-1 text-sm font-semibold whitespace-nowrap text-foreground' title={crumb.path}>{crumb.name}</span>
                            : (
                                <Button type='button' variant='ghost' size='sm' className='h-auto min-h-7 min-w-0 px-2 font-normal aria-[current=location]:font-semibold'
                                  title={crumb.path} disabled={checking} onClick={() => picker.navigate(crumb.path)}>
                                  <span className='truncate'>{crumb.name}</span>
                                </Button>
                              )}
                        </li>
                      ))}
                    </ol>
                  </nav>
                )}
            <Button ref={editButtonRef} type='button' variant='ghost' size='icon-sm' className='size-8 min-h-8' disabled={checking}
              aria-label={t(editingLocation ? 'filesystem.cancelEditing' : 'filesystem.editPath')}
              title={t(editingLocation ? 'filesystem.cancelEditing' : 'filesystem.editPath')}
              onClick={() => {
                if (editingLocation) {
                  finishEditing(false);
                } else {
                  setLocationDraft(currentPath);
                  setEditingLocation(true);
                }
              }}>
              {editingLocation
                ? <X className='size-4' strokeWidth={1.75} aria-hidden='true' />
                : <SquarePen className='size-4' strokeWidth={1.75} aria-hidden='true' />}
            </Button>
            {!editingLocation && (
              <>
                <Button type='button' variant='ghost' size='icon-sm' className='size-8 min-h-8' aria-label={t('filesystem.up')} title={t('filesystem.up')}
                  disabled={checking || !parentPath || parentPath === currentPath}
                  onClick={() => parentPath && picker.navigate(parentPath)}>
                  <FolderUp className='size-4' strokeWidth={1.75} aria-hidden='true' />
                </Button>
                <Button type='button' variant='ghost' size='icon-sm' className='size-8 min-h-8' disabled={checking} aria-label={t('filesystem.refresh')} title={t('filesystem.refresh')}
                  onClick={() => picker.setQuery(current => ({
                    ...current, path: pending ? current.path : data?.path ?? current.path,
                  }))}>
                  <RefreshCw className='size-4' strokeWidth={1.75} aria-hidden='true' />
                </Button>
              </>
            )}
            <Button type='button' variant={query.show_hidden ? 'secondary' : 'ghost'} size='icon-sm'
              className='size-8 min-h-8' disabled={checking} aria-label={t('filesystem.hidden')}
              title={t('filesystem.hidden')} aria-pressed={query.show_hidden ?? false}
              onClick={() => picker.setQuery(current => ({
                ...current, path: pending ? current.path : data?.path ?? current.path,
                show_hidden: !current.show_hidden, offset: 0,
              }))}>
              {query.show_hidden ? <Eye strokeWidth={1.75} aria-hidden='true' /> : <EyeOff strokeWidth={1.75} aria-hidden='true' />}
            </Button>
          </div>
          <div className='flex min-w-0 items-center gap-2 lg:gap-3'>
            <Field className='min-w-0 flex-1 lg:w-48 lg:flex-none'>
              <FieldLabel htmlFor={`${id}-search`} className='sr-only'>{t('filesystem.search')}</FieldLabel>
              <InputGroup>
                <InputGroupAddon><Search className='size-3.5' strokeWidth={1.75} aria-hidden='true' /></InputGroupAddon>
                <InputGroupInput id={`${id}-search`} value={picker.search} placeholder={t('filesystem.search')} disabled={checking}
                  onChange={event => picker.setSearch(event.target.value)} onKeyDown={event => {
                    if (event.key === 'Enter') event.preventDefault();
                  }} />
              </InputGroup>
            </Field>
            <Button type='button' size='sm' className='shrink-0' aria-busy={checking}
              title={picker.selectedPath || undefined}
              disabled={blocked || editingLocation || !data || !picker.selectedPath}
              onClick={confirm}>
              {checking
                ? <Spinner data-icon='inline-start' aria-hidden='true' />
                : <Check data-icon='inline-start' aria-hidden='true' />}
              {t('filesystem.confirm')}
            </Button>
          </div>
        </div>
        {data?.fallback && <p className='text-sm text-muted-foreground' role='status'>{t('filesystem.fallback')}</p>}
        <div ref={entriesContainerRef} tabIndex={-1} role='region' aria-label={t('filesystem.entries')}
          className='h-[min(240px,35dvh)] shrink-0 overflow-y-auto border-y border-border/60 py-2 outline-none sm:h-[min(320px,45dvh)]' aria-busy={pending}>
          {pending
            ? (
                <div className='flex h-full items-center justify-center gap-2' role='status'>
                  <Spinner />
                  {t('filesystem.loading')}
                </div>
              )
            : error != null
              ? <FieldError className='flex h-full items-center justify-center px-4 text-center'>{errorMessage}</FieldError>
              : !data || data.items.length === 0
                  ? <Empty className='h-full'><EmptyHeader><EmptyTitle>{t('filesystem.empty')}</EmptyTitle></EmptyHeader></Empty>
                  : (
                      <ul ref={listRef} aria-label={t('filesystem.entries')} className='flex flex-col gap-1' onKeyDown={event => {
                        if (event.key === 'ArrowLeft' && parentPath && parentPath !== currentPath) {
                          event.preventDefault();
                          enterDirectory(parentPath);
                          return;
                        }
                        if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
                        const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button[data-path-entry]:not(:disabled)')];
                        const current = (document.activeElement as HTMLElement)?.closest('li')?.querySelector<HTMLButtonElement>('button[data-path-entry]');
                        const index = current ? buttons.indexOf(current) : -1;
                        const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : index + (event.key === 'ArrowDown' ? 1 : -1);
                        event.preventDefault();
                        buttons[Math.max(0, Math.min(buttons.length - 1, next))]?.focus();
                      }}>
                        {data.items.map(entry => {
                          const selected = picker.selectedEntry?.path === entry.path;
                          const Icon = entry.kind === 'directory' ? Folder : entry.kind === 'socket' ? Plug : File;
                          return (
                            <li key={entry.path} className='flex min-w-0 items-center gap-1'>
                              <Button type='button' variant='ghost' data-path-entry className='server-path-picker__entry h-auto min-h-10 min-w-0 flex-1 justify-start gap-3 px-2.5 py-2' title={entry.path}
                                aria-label={`${entry.name}${entry.kind === 'directory' ? '/' : ''}`} aria-pressed={selected}
                                disabled={blocked || editingLocation || !canSelect(entry)}
                                onClick={() => picker.select(entry)}
                                onDoubleClick={() => {
                                  if (entry.kind === 'directory') enterDirectory(entry.path);
                                }}
                                onKeyDown={event => {
                                  if (entry.kind === 'directory' && (event.key === 'Enter' || event.key === 'ArrowRight')) {
                                    event.preventDefault();
                                    enterDirectory(entry.path);
                                  }
                                }}>
                                <Icon data-icon='inline-start' className={entry.kind === 'directory' ? 'text-primary/80' : 'text-muted-foreground'} aria-hidden='true' />
                                <span className={entry.kind === 'directory' ? 'truncate font-medium' : 'truncate font-normal'}>{entry.name}</span>
                                {entry.symlink && <Link data-icon='inline-end' aria-label={t('filesystem.symlink')} />}
                                {entry.kind === 'directory' && <ChevronRight className='ml-auto text-muted-foreground/50' aria-hidden='true' />}
                                {!entry.available && <span className='sr-only'>{t('filesystem.unavailable')}</span>}
                              </Button>
                            </li>
                          );
                        })}
                      </ul>
                    )}
        </div>
        <ListPagination page={Math.floor((data?.offset ?? 0) / (query.limit ?? 10)) + 1}
          pages={Math.max(1, Math.ceil((data?.total ?? 0) / (query.limit ?? 10)))}
          pageSize={query.limit ?? 10} disabled={blocked}
          onPageChange={page => picker.setQuery(current => ({
            ...current, path: data?.path ?? current.path, offset: (page - 1) * (current.limit ?? 10),
          }))}
          onPageSizeChange={limit => picker.setQuery(current => ({
            ...current, path: pending ? current.path : data?.path ?? current.path, limit, offset: 0,
          }))} />
        <p className='sr-only' role='status'>{checking ? t('filesystem.checking') : ''}</p>
      </div>
    </>
  );
}
