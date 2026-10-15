<script lang="ts">
	import MerchantActions from '../../../../lib/components/MerchantActions.svelte';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../../lib/components/StateBadge.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });

	// Kartu ringkasan = filter. Warna aksen mengikuti makna status (bukan hanya warna: selalu ada label teks).
	const tiles = [
		{ key: 'submitted', icon: 'M4 4h16v12H5.2L4 17.2V4Z M8 9h8 M8 12h5', accent: 'text-amber-600 bg-amber-500/10', bar: 'bg-amber-500' },
		{ key: 'under_review', icon: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14Z M21 21l-4.3-4.3', accent: 'text-sky-600 bg-sky-500/10', bar: 'bg-sky-500' },
		{ key: 'action_required', icon: 'M12 9v4 M12 17h.01 M10.3 3.9 2.5 18a2 2 0 0 0 1.7 3h15.6a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z', accent: 'text-orange-600 bg-orange-500/10', bar: 'bg-orange-500' },
		{ key: 'approved', icon: 'M12 3 4 6v6c0 4.5 3.2 8 8 9 4.8-1 8-4.5 8-9V6l-8-3Z M9 12l2.2 2.2L15.5 10', accent: 'text-emerald-600 bg-emerald-500/10', bar: 'bg-emerald-500' },
		{ key: 'rejected', icon: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z M9 9l6 6 M15 9l-6 6', accent: 'text-red-600 bg-red-500/10', bar: 'bg-red-500' },
		{ key: 'suspended', icon: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z M10 9v6 M14 9v6', accent: 'text-violet-600 bg-violet-500/10', bar: 'bg-violet-500' }
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
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<!-- hero -->
<section class="hero relative mb-5 overflow-hidden rounded-3xl bg-linear-to-br from-merah-gelap via-merah-tua to-merah p-6 text-white shadow-lg sm:p-8">
	<div class="pointer-events-none absolute -right-16 -top-20 size-72 rounded-full bg-emas/25 blur-3xl"></div>
	<div class="pointer-events-none absolute -bottom-24 left-1/3 size-64 rounded-full bg-white/10 blur-3xl"></div>
	<svg class="pointer-events-none absolute inset-0 size-full opacity-[0.07]" aria-hidden="true"><defs><pattern id="mp-dots" width="22" height="22" patternUnits="userSpaceOnUse"><circle cx="2" cy="2" r="1.6" fill="#fff" /></pattern></defs><rect width="100%" height="100%" fill="url(#mp-dots)" /></svg>
	<div class="relative flex flex-wrap items-end justify-between gap-5">
		<div class="max-w-xl">
			<span class="inline-flex items-center gap-2 rounded-full bg-white/15 px-3 py-1 text-[11px] font-bold tracking-wide backdrop-blur">
				<span class="size-1.5 animate-denyut rounded-full bg-emas"></span> {t('fm.badge')}
			</span>
			<h1 class="mt-3 font-display text-3xl font-extrabold leading-tight sm:text-4xl">{t('onb.queue')}</h1>
			<p class="mt-2 text-[13px] leading-relaxed text-white/75">{t('fm.hero')}</p>
		</div>
		<div class="rounded-2xl bg-white/10 px-5 py-3 text-center backdrop-blur">
			<div class="font-display text-4xl font-extrabold leading-none">{total}</div>
			<div class="mt-1 text-[11px] font-semibold uppercase tracking-wider text-white/70">{t('fm.total')}</div>
		</div>
	</div>
</section>

<!-- kartu ringkasan = filter -->
<nav class="mb-5 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6" aria-label={t('fin.filter')}>
	{#each tiles as tile, i (tile.key)}
		{@const active = data.status === tile.key}
		<a
			href={`?status=${tile.key}`}
			aria-current={active ? 'true' : undefined}
			style="animation-delay:{i * 60}ms"
			class="tile group relative animate-naik overflow-hidden rounded-2xl border bg-surface p-4 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md focus-visible:ring-4 focus-visible:ring-emas/60
				{active ? 'border-merah ring-2 ring-merah/20' : 'border-ink/10'}"
		>
			<span class="absolute inset-x-0 top-0 h-1 {tile.bar} {active ? 'opacity-100' : 'opacity-40 group-hover:opacity-100'} transition"></span>
			<span class="grid size-9 place-items-center rounded-xl {tile.accent}">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d={tile.icon} /></svg>
			</span>
			<span class="mt-3 block font-display text-3xl font-extrabold leading-none">{data.counts[tile.key]}</span>
			<span class="muted mt-1 block text-[11px] font-bold">{t(`onb.st.${tile.key}` as const)}</span>
		</a>
	{/each}
</nav>

<div class="relative mb-4">
	<svg viewBox="0 0 24 24" class="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-ink/40" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
	<input
		type="search"
		bind:value={q}
		aria-label={t('fm.searchLabel')}
		placeholder={t('fm.searchPh')}
		autocomplete="off"
		class="inp w-full rounded-xl py-2.5 pl-10 pr-24"
	/>
	{#if q}
		<span class="muted pointer-events-none absolute right-12 top-1/2 -translate-y-1/2 text-[11px] font-bold">{t('fm.results', { n: shown.length })}</span>
		<button type="button" aria-label={t('fm.clear')} class="absolute right-2 top-1/2 grid size-7 -translate-y-1/2 place-items-center rounded-full hover:bg-ink/10" onclick={() => (q = '')}>
			<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12" /></svg>
		</button>
	{/if}
</div>

<p class="muted mb-3 text-[11px]">{t('onb.gatewayNote')}</p>

{#if data.merchants === null}
	<p class="card p-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
{:else if shown.length === 0}
	<div class="card grid place-items-center gap-3 border-dashed px-6 py-14 text-center">
		<span class="empty-float grid size-16 place-items-center rounded-full bg-merah/10 text-merah">
			<svg viewBox="0 0 24 24" class="size-8" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7l9-4 9 4-9 4-9-4Z M3 12l9 4 9-4 M3 17l9 4 9-4" /></svg>
		</span>
		<div>
			{#if q}
				<p class="text-sm font-bold">{t('fm.noResult', { q })}</p>
				<p class="muted mt-1 text-xs">{t('fm.tryOther')}</p>
			{:else}
				<p class="text-sm font-bold">{t('onb.none')}</p>
				<p class="muted mt-1 text-xs">{t('fm.noneStatus', { status: t(`onb.st.${data.status as (typeof tiles)[number]["key"]}` as const) })}</p>
			{/if}
		</div>
	</div>
{:else}
	<ul class="grid list-none gap-4 lg:grid-cols-2">
		{#each shown as m, i (m.id)}
			<li style="animation-delay:{Math.min(i, 8) * 50}ms" class="card animate-naik p-4 transition hover:shadow-md sm:p-5">
				<div class="flex items-start gap-3">
					<span class="grid size-12 shrink-0 place-items-center rounded-2xl bg-linear-to-br from-merah to-merah-tua font-display text-lg font-extrabold text-white shadow-sm">{initials(m.display_name)}</span>
					<div class="min-w-0 flex-1">
						<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
							<h3 class="truncate text-sm font-extrabold">{m.display_name}</h3>
							<StateBadge kind="onb" value={m.status} />
						</div>
						<p class="muted mt-0.5 text-[11px]">
							{m.account_type ? t(`onb.type.${m.account_type}` as const) : '—'} · <code>{m.id.slice(0, 8)}</code> · {t('fm.registered', { date: fmt(m.created_at) })}
						</p>
					</div>
				</div>

				<dl class="mt-4 grid gap-2 rounded-xl bg-ink/[0.03] p-3 text-xs sm:grid-cols-2">
					<div class="min-w-0">
						<dt class="muted text-[10px] font-bold uppercase tracking-wide">{t('onb.owner')}</dt>
						<dd class="mt-0.5 truncate font-semibold">{m.owner_name ?? '—'}</dd>
					</div>
					<div class="min-w-0">
						<dt class="muted text-[10px] font-bold uppercase tracking-wide">{t('fm.contact')}</dt>
						<dd class="mt-0.5 truncate">{m.owner_email ?? '—'}</dd>
						{#if m.contact_phone}<dd class="muted truncate text-[11px]">{m.contact_phone}</dd>{/if}
					</div>
				</dl>

				<div class="mt-3 flex flex-wrap gap-2">
					<span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-bold {m.owner_email_verified ? 'bg-emerald-500/10 text-emerald-700' : 'bg-amber-500/10 text-amber-700'}">
						<span class="size-1.5 rounded-full bg-current"></span>{m.owner_email_verified ? t('onb.emailOk') : t('onb.emailNo')}
					</span>
					<span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-bold {m.identity_complete ? 'bg-emerald-500/10 text-emerald-700' : 'bg-amber-500/10 text-amber-700'}">
						<span class="size-1.5 rounded-full bg-current"></span>{m.identity_complete ? t('onb.identityOk') : t('onb.identityNo')}
					</span>
				</div>

				{#if m.review_note}
					<p class="mt-3 rounded-xl border-l-4 border-emas bg-emas/10 px-3 py-2 text-[11px] leading-relaxed">
						<b>{t('fm.note')}</b> {m.review_note}{#if m.reviewed_at}<span class="muted"> · {fmt(m.reviewed_at)}</span>{/if}
					</p>
				{/if}

				<div class="mt-4 flex flex-wrap items-center justify-end gap-2 border-t border-ink/10 pt-3">
					<a class="btn btn-light" href={`/dashboard/finance/merchants/${encodeURIComponent(m.id)}`}>
						<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></svg>
						{t('onb.detail')}
					</a>
					<MerchantActions {m} canReview={data.canReview} canApprove={data.canApprove} />
				</div>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.hero { animation: hero-in 0.6s cubic-bezier(0.2, 0.8, 0.2, 1) both; }
	.empty-float { animation: float 3s ease-in-out infinite; }
	@keyframes hero-in { from { opacity: 0; transform: translateY(-10px) scale(0.99); } to { opacity: 1; transform: none; } }
	@keyframes float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-6px); } }
	@media (prefers-reduced-motion: reduce) {
		.hero, .empty-float, .tile { animation: none; }
	}
</style>
