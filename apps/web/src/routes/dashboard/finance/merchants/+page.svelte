<script lang="ts">
	import { page } from '$app/state';
	import MerchantActions from '../../../../lib/components/MerchantActions.svelte';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../../lib/components/StateBadge.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });

	// Tab status = filter. Warna mengikuti makna status (selalu ada label teks).
	const tiles = [
		{ key: 'submitted', bar: 'bg-amber-500' },
		{ key: 'under_review', bar: 'bg-sky-500' },
		{ key: 'action_required', bar: 'bg-orange-500' },
		{ key: 'approved', bar: 'bg-emerald-500' },
		{ key: 'rejected', bar: 'bg-red-500' },
		{ key: 'suspended', bar: 'bg-violet-500' }
	] as const;
	const total = $derived(Object.values(data.counts as Record<string, number>).reduce((a, b) => a + b, 0));
	// Pencarian instan di daftar status terpilih: nama usaha, pemilik, email, telepon, atau ID.
	let q = $state('');
	const norm = (v: string | null | undefined) => (v ?? '').toLowerCase();
	const shown = $derived.by(() => {
		const list = data.merchants ?? [];
		const needle = q.trim().toLowerCase();
		if (!needle) return list;
		return list.filter((m: any) =>
			[m.display_name, m.legal_name, m.owner_name, m.owner_email, m.contact_phone, m.id].some((f) => norm(f).includes(needle))
		);
	});
	const initials = (n: string) =>
		n
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0]!.toUpperCase())
			.join('') || '?';
	const rp = (n: number) => (total ? Math.round((n / total) * 100) : 0);
	const base = $derived(page.url.pathname.startsWith('/platform') ? '/platform/finance' : '/dashboard/finance');
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<!-- judul + total + pencarian -->
<header class="mb-4 flex flex-wrap items-end justify-between gap-3">
	<div>
		<p class="text-[10px] font-bold uppercase tracking-[.2em] text-merah">{t('fm.badge')}</p>
		<h1 class="font-display text-2xl font-extrabold">{t('onb.queue')}</h1>
	</div>
	<div class="relative w-full sm:w-80">
		<svg viewBox="0 0 24 24" class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink/40" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
		<input type="search" bind:value={q} aria-label={t('fm.searchLabel')} placeholder={t('fm.searchPh')} autocomplete="off" class="inp w-full py-2 pl-9 pr-8" />
		{#if q}
			<button type="button" aria-label={t('fm.clear')} class="absolute right-1.5 top-1/2 grid size-6 -translate-y-1/2 place-items-center rounded-full hover:bg-ink/10" onclick={() => (q = '')}>
				<svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12" /></svg>
			</button>
		{/if}
	</div>
</header>

<!-- papan status: komposisi + filter -->
<section class="card mb-4 overflow-hidden">
	<div class="flex h-1.5 w-full bg-ink/5" aria-hidden="true">
		{#each tiles as tile (tile.key)}
			{#if data.counts[tile.key]}<span class="{tile.bar} h-full" style="width:{(data.counts[tile.key] / total) * 100}%"></span>{/if}
		{/each}
	</div>
	<nav class="grid grid-cols-3 divide-x divide-y divide-ink/10 sm:grid-cols-6 sm:divide-y-0" aria-label={t('fin.filter')}>
		{#each tiles as tile (tile.key)}
			{@const active = data.status === tile.key}
			<a
				href={`?status=${tile.key}`}
				aria-current={active ? 'true' : undefined}
				class="relative px-3 py-2.5 transition hover:bg-gading focus-visible:ring-4 focus-visible:ring-emas/60 {active ? 'bg-gading' : ''}"
			>
				{#if active}<span class="absolute inset-x-0 bottom-0 h-0.5 bg-merah"></span>{/if}
				<span class="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-wide {active ? 'text-ink' : 'muted'}">
					<span class="size-1.5 rounded-full {tile.bar}"></span>{t(`onb.st.${tile.key}` as const)}
				</span>
				<span class="mt-1 flex items-baseline gap-1.5">
					<span class="font-display text-xl font-extrabold tabular-nums leading-none">{data.counts[tile.key]}</span>
					<span class="muted text-[10px] tabular-nums">{rp(data.counts[tile.key])}%</span>
				</span>
			</a>
		{/each}
	</nav>
	<p class="muted border-t border-ink/10 px-3 py-1.5 text-[10px]">
		{t('fm.total')}: <b class="tabular-nums text-ink">{total}</b> · {t('onb.gatewayNote')}
		{#if q}· {t('fm.results', { n: shown.length })}{/if}
	</p>
</section>

{#if data.merchants === null}
	<p class="card p-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
{:else if shown.length === 0}
	<div class="card px-6 py-12 text-center">
		{#if q}
			<p class="text-sm font-bold">{t('fm.noResult', { q })}</p>
			<p class="muted mt-1 text-xs">{t('fm.tryOther')}</p>
		{:else}
			<p class="text-sm font-bold">{t('onb.none')}</p>
			<p class="muted mt-1 text-xs">{t('fm.noneStatus', { status: t(`onb.st.${data.status as (typeof tiles)[number]["key"]}` as const) })}</p>
		{/if}
	</div>
{:else}
	<div class="card overflow-hidden">
		<div class="overflow-x-auto">
			<table class="w-full min-w-[900px] text-left text-xs">
				<thead class="bg-gading text-[10px] font-bold uppercase tracking-wider text-merah-gelap/60">
					<tr>
						<th class="px-3 py-2.5">{t('onb.queue')}</th>
						<th class="px-3 py-2.5">{t('onb.owner')}</th>
						<th class="px-3 py-2.5">{t('fm.contact')}</th>
						<th class="px-3 py-2.5">Status</th>
						<th class="px-3 py-2.5">KYC</th>
						<th class="px-3 py-2.5 text-right"></th>
					</tr>
				</thead>
				<tbody class="divide-y divide-ink/10">
					{#each shown as m (m.id)}
						<tr class="align-top hover:bg-gading/60">
							<td class="px-3 py-2.5">
								<div class="flex items-center gap-2.5">
									<span class="grid size-8 shrink-0 place-items-center rounded-lg bg-merah-gelap font-display text-[11px] font-extrabold text-emas">{initials(m.display_name)}</span>
									<div class="min-w-0">
										<p class="truncate text-[13px] font-extrabold">{m.display_name}</p>
										<p class="muted truncate text-[10px]">
											{m.account_type ? t(`onb.type.${m.account_type}` as const) : '—'} · <code class="font-mono">{m.id.slice(0, 8)}</code> · {fmt(m.created_at)}
										</p>
									</div>
								</div>
							</td>
							<td class="px-3 py-2.5 font-semibold">{m.owner_name ?? '—'}</td>
							<td class="px-3 py-2.5">
								<p class="max-w-52 truncate">{m.owner_email ?? '—'}</p>
								{#if m.contact_phone}<p class="muted text-[10px]">{m.contact_phone}</p>{/if}
							</td>
							<td class="px-3 py-2.5">
								<StateBadge kind="onb" value={m.status} />
								{#if m.review_note}
									<p class="muted mt-1 line-clamp-2 max-w-48 text-[10px]" title={m.review_note}>
										<b>{t('fm.note')}</b> {m.review_note}{#if m.reviewed_at} · {fmt(m.reviewed_at)}{/if}
									</p>
								{/if}
							</td>
							<td class="px-3 py-2.5">
								<div class="flex flex-col gap-1">
									<span class="inline-flex items-center gap-1 whitespace-nowrap text-[10px] font-bold {m.owner_email_verified ? 'text-emerald-700' : 'text-amber-700'}">
										<span class="size-1.5 rounded-full bg-current"></span>{m.owner_email_verified ? t('onb.emailOk') : t('onb.emailNo')}
									</span>
									<span class="inline-flex items-center gap-1 whitespace-nowrap text-[10px] font-bold {m.identity_complete ? 'text-emerald-700' : 'text-amber-700'}">
										<span class="size-1.5 rounded-full bg-current"></span>{m.identity_complete ? t('onb.identityOk') : t('onb.identityNo')}
									</span>
								</div>
							</td>
							<td class="px-3 py-2.5">
								<div class="flex items-center justify-end gap-2">
									<a class="btn btn-light" href={`${base}/merchants/${encodeURIComponent(m.id)}`}>{t('onb.detail')}</a>
									<MerchantActions {m} canReview={data.canReview} canApprove={data.canApprove} />
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
{/if}
