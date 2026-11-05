import { useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { ArrowLeft, CirclePlus, RefreshCw, Search } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';

import { SubscriptionNodeGrid } from './subscription-node-grid';
import { SubscriptionNodeEditor } from './subscription-node-editor';
import { useSubscriptionSources } from './use-subscription-sources';
import { SubscriptionContentState } from './subscription-content-state';

export function SubscriptionSourcePanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t, i18n } = useTranslation();
  const addSourceRef = useRef<HTMLButtonElement>(null);
  const {
    sources,
    refreshingSources,
    busyNodes,
    refreshing,
    sourceLoading,
    nodeLoading,
    nodes,
    selected,
    setSelected,
    tab,
    setTab,
    search,
    setSearch,
    size,
    setSize,
    setPage,
    error,
    busy,
    editor,
    setEditor,
    form,
    setForm,
    creating,
    setCreating,
    formError,
    setFormError,
    confirmNavigation,
    reload,
    openSource,
    openSettings,
    refresh,
    saveSource,
    toggle,
    reorderNodes,
    displayedSources,
    inManualCollection,
    current,
    pages,
    newSource,
  } = useSubscriptionSources(active);
  const openNode = useCallback((node: import('@/api/api-client').SubscriptionNodeSummary) => setEditor({ node }), [setEditor]);
  const fields = form && (
    <div className='grid grid-cols-[minmax(120px,1fr)_minmax(200px,2fr)] items-center gap-x-6 gap-y-(--form-field-gap) pt-2 pb-(--form-field-gap) max-[720px]:grid-cols-[minmax(100px,1fr)_minmax(0,2fr)] max-[720px]:gap-3 max-[480px]:grid-cols-1 max-[480px]:gap-2 [&_[aria-invalid=true]]:outline-1 [&_[aria-invalid=true]]:outline-[var(--color-danger)] [&_[data-slot=select-trigger]]:w-full [&_input:not([type=checkbox])]:w-full [&_input:not([type=checkbox])]:min-w-0 [&_input[type=checkbox]]:justify-self-start [&_textarea]:max-h-45 [&_textarea]:min-h-[90px] [&_textarea]:resize-y [&_textarea]:rounded-[var(--radius-control)] [&_textarea]:border-0 [&_textarea]:bg-[var(--color-surface-raised)] [&_textarea]:p-3'>
      <label htmlFor='source-name'>{t('subscriptions.common.name')}</label>
      <Input
        disabled={busy}
        id='source-name'
        onChange={(event) => setForm({ ...form, name: event.target.value })}
        value={form.name}
      />
      {(!form.source || form.source.source_kind === 'remote') && (
        <>
          <label htmlFor='source-url'>{t('subscriptions.sources.url')}</label>
          <Input
            autoComplete='off'
            disabled={busy}
            id='source-url'
            onChange={(event) => setForm({ ...form, url: event.target.value })}
            placeholder='https://'
            type='url'
            value={form.url}
          />
          <label htmlFor='source-format'>{t('subscriptions.channel.field.format')}</label>
          <SelectField
            disabled={busy}
            id='source-format'
            onValueChange={(value) => setForm({ ...form, format: value })}
            value={form.format}
            items={[
              { value: 'auto', label: t('subscriptions.sources.auto') },
              { value: 'sing-box-json', label: 'sing-box JSON' },
              { value: 'mihomo-yaml', label: 'Mihomo YAML' },
              { value: 'uri-list', label: 'URI' },
            ]}
          />
          <label htmlFor='source-interval'>{t('subscriptions.sources.interval')}</label>
          <SelectField
            disabled={busy}
            id='source-interval'
            onValueChange={(value) => setForm({ ...form, interval: value })}
            value={form.interval}
            items={[...new Set(['0', '60', '360', '720', '1440', form.interval])].map((value) => ({
              value,
              label: value === '0' ? t('subscriptions.sources.onDemand') : `${value} min`,
            }))}
          />
        </>
      )}
    </div>
  );

  return (
    <Tabs
      className='flex h-full min-h-0 min-w-0 flex-1 flex-col'
      value={tab}
      onValueChange={(value) => value === 'settings' ? void openSettings() : confirmNavigation(() => setTab('nodes'))}
    >
      {error != null && <ErrorNotice error={error} title={t('subscriptions.source.loadFailed')} />}
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='flex w-full min-w-0 flex-1 items-center justify-end gap-4 max-[900px]:gap-2 has-[[data-detail-tabs]]:flex-wrap'>
          {selected && selected !== 'manual' && (
            <div className='me-auto flex min-w-0 flex-none items-center gap-2 max-[900px]:gap-0.5' data-detail-tabs>
              <TabsList className='max-w-full flex-none justify-start self-start overflow-x-auto [scrollbar-width:none] *:data-[slot=tabs-trigger]:flex-none' aria-label={t('subscriptions.sources.settings')}>
                <TabsTrigger value='nodes' disabled={busy}>
                  {t('subscriptions.sources.nodes')}
                </TabsTrigger>
                <TabsTrigger value='settings' disabled={busy}>
                  {t('subscriptions.sources.settings')}
                </TabsTrigger>
              </TabsList>
            </div>
          )}
          <div className='flex min-w-0 flex-none items-center gap-2 ms-auto *:data-[slot=button]:border-transparent *:data-[slot=button]:bg-transparent'>
            {selected && (
              <Button
                aria-label={t('subscriptions.sources.back')}
                title={t('subscriptions.sources.back')}
                disabled={busy}
                onClick={() => confirmNavigation(() => {
                  setSelected(null);
                  setSearch('');
                  setForm(null);
                  setFormError('');
                  setTab('nodes');
                })}
                size='icon-sm'
                variant='ghost'
              >
                <ArrowLeft aria-hidden='true' />
              </Button>
            )}
            {(tab === 'nodes' || !selected) && (
              <>
                <Button
                  aria-label={t('subscriptions.sources.refresh')}
                  title={t('subscriptions.sources.refresh')}
                  disabled={refreshing || Object.values(refreshingSources).includes('pending')}
                  onClick={() =>
                    selected === 'manual'
                      ? reload()
                      : void refresh(
                        selected
                          ? [selected]
                          : sources
                              .filter((source) => source.enabled && source.source_kind === 'remote')
                              .map((source) => source.id),
                      )
                  }
                  size='icon-sm'
                  variant='ghost'
                >
                  <RefreshCw aria-hidden='true' className={refreshing || Object.values(refreshingSources).includes('pending') ? 'animate-spin' : ''} />
                </Button>
                {(!selected || selected === 'manual') && (
                  <Button
                    ref={addSourceRef}
                    aria-label={t(
                      selected ? 'subscriptions.nodes.add' : 'subscriptions.source.attach',
                    )}
                    title={t(selected ? 'subscriptions.nodes.add' : 'subscriptions.source.attach')}
                    disabled={busy}
                    onClick={() =>
                      selected
                        ? setEditor({ node: null })
                        : (setForm(newSource()), setCreating(true), setFormError(''))
                    }
                    size='icon-sm'
                    variant='ghost'
                  >
                    <CirclePlus aria-hidden='true' />
                  </Button>
                )}
              </>
            )}
          </div>
          {(tab === 'nodes' || !selected) && (
            <InputGroup className='h-(--control-height-sm) min-w-30 w-60 max-[900px]:w-50'>
              <InputGroupAddon><Search aria-hidden='true' /></InputGroupAddon>
              <InputGroupInput className='h-full min-h-0'
                aria-label={t('subscriptions.sources.search')}
                onChange={(event) => {
                  setSearch(event.target.value);
                  setPage(1);
                }}
                placeholder={t('subscriptions.sources.search')}
                value={search}
              />
            </InputGroup>
          )}
        </div>
      </ToolbarActions>
      {selected
        ? (
            tab === 'settings' && form
              ? (
                  <TabsContent value='settings' className='flex min-h-0 min-w-0 flex-none flex-col overflow-auto [&>footer]:flex [&>footer]:justify-end [&>footer]:gap-3 [&>footer]:pt-2'>
                    {formError && (
                      <ErrorNotice error={formError} />
                    )}
                    {fields}
                    <footer>
                      <Button disabled={busy} onClick={() => void saveSource()} variant='default'>
                        {t('subscriptions.sources.save')}
                      </Button>
                    </footer>
                  </TabsContent>
                )
              : (
                  <TabsContent value='nodes' className='flex h-full min-h-0 min-w-0 flex-1 flex-col'>
                    <SubscriptionNodeGrid
                      key={selected}
                      onReorder={(ids) => reorderNodes(selected, ids)}
                      loading={nodeLoading}
                      error={Boolean(error)}
                      busy={busy}
                      busyNodes={busyNodes}
                      nodes={nodes.filter((node) =>
                        selected === 'manual' ? inManualCollection(node) : node.source_id === selected,
                      )}
                      onOpen={openNode}
                      onVisibility={toggle}
                      search={search}
                    />
                  </TabsContent>
                )
          )
        : (
            <>
              <div className='min-h-0 flex-1 overflow-auto flex flex-col *:[table]:flex-none' aria-busy={sourceLoading}>
                <table className='workspace-table w-full min-w-240 table-fixed border-collapse [&_td]:border-b [&_td]:px-4 [&_td]:text-center [&_td:first-child_button]:w-full [&_td:first-child_button]:justify-start [&_td:first-child_button]:truncate [&_td:first-child_button]:text-left [&_td:last-child]:whitespace-nowrap [&_td:last-child>[data-slot=button]+[data-slot=button]]:ms-2 [&_th]:bg-[var(--color-surface-raised)] [&_th]:px-4 [&_th]:text-center [&_th]:text-muted-foreground [&_tr>:first-child]:w-[32%] [&_tr>:first-child]:text-left [&_tr>:nth-child(2)]:w-[16%] [&_tr>:nth-child(3)]:w-[20%] [&_tr>:nth-child(4)]:w-[32%] max-[900px]:[&_td]:px-2 max-[900px]:[&_th]:px-2'>
                  <thead>
                    <tr>
                      <th>{t('subscriptions.tabs.sources')}</th>
                      <th>{t('subscriptions.sources.nodes')}</th>
                      <th>{t('subscriptions.sources.updated')}</th>
                      <th>{t('subscriptions.common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {!sourceLoading && displayedSources.slice((current - 1) * size, current * size).map((source) => (
                      <tr key={source.id}>
                        <td>
                          <span className='block truncate' title={source.name}>
                            {source.name}
                          </span>
                        </td>
                        <td>
                          {
                            nodeLoading
                              ? '—'
                              : nodes.filter((node) =>
                                source.id === 'manual'
                                  ? inManualCollection(node)
                                  : node.source_id === source.id,
                              ).length
                          }
                        </td>
                        <td>
                          {source.updated_at
                            ? new Intl.DateTimeFormat(i18n.language, {
                                dateStyle: 'short',
                                timeStyle: 'short',
                              }).format(new Date(source.updated_at))
                            : '—'}
                        </td>
                        <td>
                          <Button size='sm' disabled={busy} onClick={() => openSource(source.id)} variant='outline'>
                            {t('subscriptions.sources.edit')}
                          </Button>
                          <Button
                            size='sm'
                            disabled={
                              refreshingSources[source.id] === 'pending' || (source.id !== 'manual' && source.source_kind !== 'remote')
                            }
                            onClick={() =>
                              source.id === 'manual' ? reload() : void refresh([source.id])
                            }
                            variant='outline'
                          >
                            {refreshingSources[source.id] === 'success' ? t('subscriptions.sources.refreshed') : refreshingSources[source.id] === 'error' ? t('subscriptions.sources.refreshFailed') : t('subscriptions.sources.refresh')}
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {sourceLoading
                  ? <SubscriptionContentState loading />
                  : !displayedSources.length && !error && <SubscriptionContentState kind={search ? 'search' : 'sources'} title={t(search ? 'subscriptions.sources.noMatches' : 'subscriptions.source.empty.title')} />}
              </div>
              <ListPagination
                page={current}
                pages={pages}
                pageSize={size}
                disabled={displayedSources.length === 0}
                onPageChange={setPage}
                onPageSizeChange={value => {
                  setSize(value);
                  setPage(1);
                }}
              />
            </>
          )}
      {editor && (
        <SubscriptionNodeEditor
          candidates={nodes}
          node={editor.node}
          onClose={() => setEditor(null)}
          onSaved={reload}
        />
      )}
      <Dialog
        onOpenChange={(open) => {
          if (!open && !busy) {
            setCreating(false);
          }
        }}
        onOpenChangeComplete={(open) => {
          if (!open) setForm(current => current?.source ? current : null);
        }}
        open={creating}
      >
        <DialogContent className='w-160 max-w-[calc(100vw-32px)] sm:max-w-160' finalFocus={addSourceRef}>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.source.attach')}</DialogTitle>
            <DialogDescription className='sr-only'>
              {t('subscriptions.sources.url')}
            </DialogDescription>
          </DialogHeader>
          {formError && (
            <ErrorNotice error={formError} />
          )}
          {fields}
          <DialogFooter>
            <Button
              disabled={busy}
              onClick={() => {
                setCreating(false);
              }}
              variant='outline'
            >
              {t('common.cancel')}
            </Button>
            <Button disabled={busy} onClick={() => void saveSource()} variant='default'>
              {t('subscriptions.sources.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Tabs>
  );
}
