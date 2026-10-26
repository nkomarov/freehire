<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { ArrowRight, LoaderCircle, Search, X } from '@lucide/svelte';
  import type { RailEntry, RailSection } from '$lib/filterSections';
  import { FILTER_SEARCH_MIN_QUERY, type FilterSearchHit } from '$lib/filterSearch';
  import type { FacetCounts } from '$lib/types';
  import { focusTrap } from '$lib/actions/focusTrap';
  import { errorMessage } from '$lib/utils';
  import FilterSearchResults from './FilterSearchResults.svelte';

  // The reusable two-pane filter-modal chrome: backdrop, header, the sectioned left
  // rail, and the deferred footer (Clear all / Apply / live preview). It knows nothing
  // about jobs vs companies — a domain wrapper (FilterModal, CompanyFilterModal) passes
  // the rail, a `staged` edit store, per-entry counts, a `seed` thunk run on open, an
  // `apply` action, and a `pane` snippet that renders the active entry's controls.
  //
  // Deferred-apply contract: the pane edits the caller's staged copy; nothing reaches
  // the live list until `apply` runs on the footer button.

  // The minimal staged-store surface the shell drives; the wrapper's StagedFilters /
  // StagedCompanyFilters both satisfy it (seed/commit are closed over by the wrapper's
  // `seed`/`apply` thunks, so the shell stays domain-agnostic).
  interface StagedSurface {
    readonly active: number;
    params(): URLSearchParams;
    clear(): void;
  }

  let {
    open = false,
    onClose,
    title = 'All filters',
    rail,
    sections,
    staged,
    entryCount,
    seed,
    apply,
    applyDisabled = false,
    applyLabel,
    previewCount,
    countsFetch,
    initialKey,
    pane,
    headerAction,
    titleHint,
    extra,
    footerNote,
    searchItems,
    onSearchToggle,
    searchSelection,
    filterQuery = $bindable(''),
  }: {
    open?: boolean;
    onClose: () => void;
    title?: string;
    rail: RailEntry[];
    sections: RailSection[];
    staged: StagedSurface;
    entryCount: (e: RailEntry) => number;
    /** Seed the staged copy from the live state — run once per open. */
    seed: () => void;
    /** Commit the staged copy (or a custom save). A thrown error keeps the modal open. */
    apply: () => void | Promise<void>;
    applyDisabled?: boolean;
    /** Custom footer label (e.g. "Save"); when absent the button shows the live preview. */
    applyLabel?: string;
    /** Legacy total-only preview: the footer count. Used when `countsFetch` is absent. */
    previewCount?: (params: URLSearchParams) => Promise<number>;
    /** Live disjunctive facet counts for the staged selection. When set, the shell fetches
     *  the full distribution (debounced), drives the footer total + a loading spinner, and
     *  hands the distribution to the `pane` snippet — so every control shows live counts. */
    countsFetch?: (params: URLSearchParams) => Promise<FacetCounts>;
    /** Rail entry to land on when the modal opens (defaults to the first entry) — lets a
     *  caller keep a lead tab (e.g. "My filters") in the rail without opening on it. */
    initialKey?: string;
    /** Renders the active entry's controls into the right pane, given the live staged
     *  facet counts (null until the first fetch resolves or when `countsFetch` is absent). */
    pane: Snippet<[RailEntry, FacetCounts | null]>;
    /** Optional control rendered in the header row, between the title and Close (e.g. the
     *  job modal's "Apply my profile"). Absent for callers that don't need one. */
    headerAction?: Snippet;
    /** Optional small hint rendered right after the title (e.g. the job modal's
     *  include/exclude tooltip). Absent for callers with nothing to explain there —
     *  the companies modal has no excludable facets, so it passes none. */
    titleHint?: Snippet;
    /** Optional content above the pane (e.g. the profile editor's "import from CV"). */
    extra?: Snippet;
    /** Optional footer nudge above the action row, handed a `jumpTo` to switch panes and
     *  the current `activeKey` — lets a domain wrapper add copy (e.g. "save this search")
     *  that jumps to another tab without the shell knowing what the tabs mean. */
    footerNote?: Snippet<[{ jumpTo: (key: string) => void; activeKey: string }]>;
    searchItems?: readonly FilterSearchHit[];
    onSearchToggle?: (hit: FilterSearchHit) => void;
    searchSelection?: (hit: FilterSearchHit) => { included: boolean; excluded: boolean; excludable: boolean };
    filterQuery?: string;
  } = $props();

  // Set on the first open (see the seed effect); until then activeEntry falls back to
  // the first rail entry.
  let active = $state<string>('');

  // Seed the staged copy from the live state each time the modal opens, and land on the
  // `initialKey` entry (falling back to the first) when the current one isn't in this rail.
  let wasOpen = false;
  let searchInput = $state<HTMLInputElement | null>(null);
  const querySection = $derived.by(() => {
    const q = filterQuery.trim().toLowerCase();
    if (!searchItems || q.length < FILTER_SEARCH_MIN_QUERY) return undefined;
    const matched = rail.filter((entry) => {
      const names = [entry.label.toLowerCase()];
      for (const hit of searchItems) {
        if (hit.railKey !== entry.key || hit.param || hit.action) continue;
        if (hit.label.toLowerCase() !== entry.label.toLowerCase()) continue;
        for (const alias of hit.aliases ?? []) names.push(alias.toLowerCase());
      }
      return names.some((name) => name.startsWith(q));
    });
    return matched.length === 1 ? matched[0] : undefined;
  });
  const searching = $derived(
    searchItems != null && filterQuery.trim().length >= FILTER_SEARCH_MIN_QUERY && !querySection,
  );
  $effect(() => {
    if (open && !wasOpen) {
      seed();
      if (!rail.some((e) => e.key === active)) {
        active = (initialKey && rail.some((e) => e.key === initialKey) ? initialKey : rail[0]?.key) ?? '';
      }
      if (searchItems) void tick().then(() => searchInput?.focus());
    }
    if (!open && filterQuery) filterQuery = '';
    wasOpen = open;
  });

  function jumpTo(key: string) {
    active = key;
    filterQuery = '';
  }

  const shownKey = $derived(querySection?.key ?? active);
  const activeEntry = $derived(querySection ?? rail.find((e) => e.key === active) ?? rail[0]);

  // Debounced live preview for the staged filters, keyed by a monotonic gen so a slow
  // response can't overwrite a newer one. With `countsFetch` the shell holds the full
  // distribution (`liveCounts`) + a `loading` flag driving the footer spinner; otherwise
  // it falls back to the total-only `previewCount`.
  let showCount = $state<number | null>(null);
  let liveCounts = $state<FacetCounts | null>(null);
  let loading = $state(false);
  let countGen = 0;
  $effect(() => {
    if (!open) return;
    const params = staged.params(); // tracks the staged edits
    const gen = ++countGen;
    let t: ReturnType<typeof setTimeout> | undefined;
    if (countsFetch) {
      const cf = countsFetch;
      loading = true;
      t = setTimeout(() => {
        cf(params)
          .then((c) => {
            if (gen === countGen) {
              liveCounts = c;
              loading = false;
            }
          })
          .catch(() => {
            if (gen === countGen) loading = false;
          });
      }, 200);
    } else if (previewCount) {
      const pc = previewCount;
      t = setTimeout(() => {
        pc(params)
          .then((n) => {
            if (gen === countGen) showCount = n;
          })
          .catch(() => {});
      }, 200);
    }
    return () => clearTimeout(t);
  });

  // The footer total: the live distribution's total when count-fetching, else the
  // legacy preview number.
  const total = $derived(countsFetch ? liveCounts?.total ?? null : showCount);

  let applyBusy = $state(false);
  let applyError = $state<string | null>(null);

  async function runApply() {
    if (applyDisabled || applyBusy) return;
    applyBusy = true;
    applyError = null;
    try {
      await apply();
      onClose();
    } catch (e) {
      applyError = errorMessage(e, 'Something went wrong. Please try again.');
    } finally {
      applyBusy = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return;
    if (filterQuery) {
      filterQuery = '';
      return;
    }
    onClose();
  }

  function selectionOf(hit: FilterSearchHit) {
    return searchSelection?.(hit) ?? { included: false, excluded: false, excludable: false };
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-stretch justify-center sm:items-center sm:p-6">
    <!-- backdrop -->
    <button class="absolute inset-0 bg-foreground/35" aria-label="Close filters" onclick={onClose}></button>

    <div
      class={[
        'relative flex h-full w-full flex-col overflow-hidden bg-background shadow-2xl sm:max-h-[calc(100vh-3rem)] sm:max-w-4xl sm:rounded-2xl sm:border sm:border-border',
        searching ? 'sm:h-screen' : 'sm:h-auto',
      ]}
      role="dialog"
      aria-modal="true"
      aria-label={title}
      {@attach focusTrap()}
    >
      <!-- header -->
      <div class="flex items-center gap-3 border-b border-border px-4 py-3">
        <h2 class="flex shrink-0 items-center gap-1.5 text-base font-semibold tracking-tight">
          {title}
          {#if titleHint}{@render titleHint()}{/if}
        </h2>
        {#if searchItems}
          <div
            class="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-input px-3 focus-within:ring-2 focus-within:ring-ring"
          >
            <Search class="size-4 shrink-0 text-muted-foreground" />
            <input
              bind:this={searchInput}
              bind:value={filterQuery}
              type="text"
              placeholder="Search filters"
              aria-label="Search filters"
              autocomplete="off"
              spellcheck="false"
              class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            {#if filterQuery}
              <button
                type="button"
                aria-label="Clear filter search"
                onclick={() => (filterQuery = '')}
                class="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              >
                <X class="size-3.5" />
              </button>
            {/if}
          </div>
        {:else}
          <div class="flex-1"></div>
        {/if}
        {#if searchItems && headerAction}
          <div class="mx-2 h-6 w-px shrink-0 bg-border" aria-hidden="true"></div>
        {/if}
        {#if headerAction}{@render headerAction()}{/if}
        <button
          type="button"
          onclick={onClose}
          aria-label="Close"
          class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <div class="flex min-h-0 flex-1">
        <!-- rail -->
        <nav class="w-40 shrink-0 overflow-y-auto border-r border-border py-2 sm:w-56">
          {#each sections as section (section)}
            {@const entries = rail.filter((e) => e.section === section)}
            {#if entries.length}
              <div class="px-3 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{section}</div>
              {#each entries as e (e.key)}
                {@const n = entryCount(e)}
                <button
                  type="button"
                  onclick={() => jumpTo(e.key)}
                  class={[
                    'flex w-full items-center justify-between gap-2 border-l-2 px-3 py-2 text-left text-sm transition-colors',
                    shownKey === e.key
                      ? 'border-brand bg-accent font-semibold'
                      : 'border-transparent font-medium hover:bg-accent',
                  ]}
                >
                  <span class="truncate">{e.label}</span>
                  {#if n > 0}
                    <span
                      class={[
                        'shrink-0 rounded-full px-1.5 text-[11px] font-semibold',
                        shownKey === e.key ? 'bg-brand text-brand-foreground' : 'bg-secondary text-muted-foreground',
                      ]}>{n}</span
                    >
                  {/if}
                </button>
              {/each}
            {/if}
          {/each}
        </nav>

        <!-- pane -->
        <section class="min-w-0 flex-1 overflow-y-auto p-4 sm:p-6">
          {#if extra && !searching}<div class="mb-6">{@render extra()}</div>{/if}
          {#if searching && searchItems}
            <FilterSearchResults
              items={searchItems}
              query={filterQuery}
              counts={liveCounts}
              onToggle={(hit) => onSearchToggle?.(hit)}
              onJump={jumpTo}
              selection={selectionOf}
            />
          {:else if activeEntry}
            {@render pane(activeEntry, liveCounts)}
          {/if}
        </section>
      </div>

      <!-- footer -->
      <div class="flex flex-col gap-2 border-t border-border px-4 py-3">
        {#if applyError}
          <p class="text-sm text-destructive">{applyError}</p>
        {/if}
        <div class="flex items-center justify-between">
          <button
            type="button"
            onclick={() => staged.clear()}
            class="inline-flex h-11 items-center gap-1.5 rounded-lg px-3 -ml-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            ><X class="size-3.5" />Clear all</button
          >
          <button
            type="button"
            onclick={runApply}
            disabled={applyDisabled || applyBusy}
            class="inline-flex h-11 items-center gap-1.5 rounded-lg bg-brand px-6 text-sm font-semibold text-brand-foreground ring-2 ring-brand-ring ring-offset-2 ring-offset-background transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {#if applyLabel}
              {applyBusy ? 'Saving…' : applyLabel}
              {#if !applyBusy}<ArrowRight class="size-4" aria-hidden="true" />{/if}
            {:else if loading}
              Show <LoaderCircle class="size-4 animate-spin" /> jobs
            {:else}
              Show {total != null ? total.toLocaleString('en-US') : ''}
              {total === 1 ? 'job' : 'jobs'}
              <ArrowRight class="size-4" aria-hidden="true" />
            {/if}
          </button>
        </div>
        {#if footerNote}{@render footerNote({ jumpTo, activeKey: active })}{/if}
      </div>
    </div>
  </div>
{/if}
