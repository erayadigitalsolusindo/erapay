<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t, tx } = i18n;
	let open = $state<string | null>(null);
	let act = $state<string | null>(null);
	let filter = $state('all');
	let copied = $state<string | null>(null);

	const fmt = (s: string) => new Date(s).toLocaleDateString(i18n.locale, { day: '2-digit', month: 'short', year: 'numeric' });
	const stLabel = (s: string) => tx(`onb.st.${s}`, s);
	const inisial = (s: string) =>
		s
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0])
			.join('')
			.toUpperCase() || '?';

	// Warna status: hijau = aktif, kuning = menunggu, oranye = perlu tindakan, merah = ditolak/ditangguhkan.
	const TONE: Record<string, { pill: string; dot: string; ring: string }> = {
		approved: { pill: 'bg-emerald-500/15 text-emerald-700', dot: 'bg-emerald-500', ring: 'from-emerald-500 to-teal-600' },
		submitted: { pill: 'bg-amber-500/15 text-amber-700', dot: 'bg-amber-500', ring: 'from-amber-400 to-orange-500' },
		under_review: { pill: 'bg-sky-500/15 text-sky-700', dot: 'bg-sky-500', ring: 'from-sky-500 to-indigo-500' },
		action_required: { pill: 'bg-orange-500/15 text-orange-700', dot: 'bg-orange-500', ring: 'from-orange-500 to-red-500' },
		rejected: { pill: 'bg-red-500/15 text-red-700', dot: 'bg-red-500', ring: 'from-red-500 to-rose-700' },
		suspended: { pill: 'bg-red-500/15 text-red-700', dot: 'bg-red-500', ring: 'from-red-500 to-rose-700' },
		draft: { pill: 'bg-ink/10 text-ink/70', dot: 'bg-ink/40', ring: 'from-slate-400 to-slate-600' }
	};
	const tone = (s: string) => TONE[s] ?? TONE.draft;

	type M = { id: string; display_name: string; legal_name: string; onboarding_status: string; created_at: string };
	const all = $derived(data.merchants as M[]);
	const counts = $derived(
		all.reduce<Record<string, number>>((a, m) => ((a[m.onboarding_status] = (a[m.onboarding_status] ?? 0) + 1), a), {})
	);
	const pending = $derived((counts.submitted ?? 0) + (counts.under_review ?? 0) + (counts.action_required ?? 0));
	const blocked = $derived((counts.rejected ?? 0) + (counts.suspended ?? 0));
	const statuses = $derived(Object.keys(counts).sort((a, b) => counts[b] - counts[a]));
	const shown = $derived(filter === 'all' ? all : all.filter((m) => m.onboarding_status === filter));

	async function copy(id: string) {
		try {
			await navigator.clipboard.writeText(id);
			copied = id;
			setTimeout(() => (copied = copied === id ? null : copied), 1500);
		} catch {
			/* clipboard tidak tersedia */
		}
	}
</script>

<svelte:head><title>{t('pf.tn.pageTitle')}</title></svelte:head>

<div class="space-y-6">
	<!-- hero + pencarian -->
	<section class="relative overflow-hidden rounded-3xl bg-linear-to-br from-merah-gelap via-merah-tua to-merah p-6 text-white shadow-lg sm:p-8">
		<div class="pointer-events-none absolute -right-16 -top-20 size-64 rounded-full bg-emas/25 blur-3xl"></div>
		<div class="pointer-events-none absolute -bottom-24 left-1/3 size-56 rounded-full bg-white/10 blur-3xl"></div>
		<div class="relative flex flex-wrap items-end justify-between gap-5">
			<div class="max-w-2xl">
				<p class="text-[10px] font-bold uppercase tracking-[.25em] text-white/60">DEWA · Platform</p>
				<h1 class="mt-1 font-display text-3xl font-extrabold">{t('pf.nav.tenants')}</h1>
				<p class="mt-2 text-[13px] leading-relaxed text-white/75">
					{t('pf.tn.sub')}
					<a href="/platform/audit" class="font-bold text-emas underline decoration-emas/40 underline-offset-2 hover:decoration-emas">{t('pf.tn.subLink')}</a>
				</p>
			</div>
			<form method="GET" class="flex w-full gap-2 sm:w-auto">
				<div class="relative min-w-0 flex-1 sm:w-80">
					<svg viewBox="0 0 24 24" class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-white/50" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
					<input
						name="q"
						value={data.q}
						placeholder={t('pf.tn.searchPh')}
						class="w-full rounded-full border border-white/20 bg-white/10 py-2.5 pl-9 pr-4 text-sm text-white outline-none backdrop-blur placeholder:text-white/50 focus:border-emas focus:bg-white/15"
					/>
				</div>
				<button class="rounded-full bg-emas px-5 py-2.5 text-xs font-bold text-merah-gelap transition hover:brightness-110">{t('pf.tn.search')}</button>
			</form>
		</div>
	</section>

	{#if data.error}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>{/if}

	<!-- ringkasan -->
	<section class="grid grid-cols-2 gap-3 lg:grid-cols-4">
		{#each [
			{ label: tx('pf.tn.stTotal', 'Total tenant'), n: all.length, cls: 'from-merah to-merah-tua', icon: 'M3 21h18M5 21V7l7-4 7 4v14M9 21v-6h6v6' },
			{ label: stLabel('approved'), n: counts.approved ?? 0, cls: 'from-emerald-500 to-teal-600', icon: 'm5 12 5 5L20 7' },
			{ label: tx('pf.tn.stPending', 'Dalam proses'), n: pending, cls: 'from-amber-400 to-orange-500', icon: 'M12 6v6l4 2M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z' },
			{ label: tx('pf.tn.stBlocked', 'Ditolak / ditangguhkan'), n: blocked, cls: 'from-red-500 to-rose-700', icon: 'M12 9v4m0 4h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z' }
		] as s (s.label)}
			<div class="card flex items-center gap-3.5 p-4">
				<span class="grid size-11 shrink-0 place-items-center rounded-2xl bg-linear-to-br {s.cls} text-white shadow-sm">
					<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d={s.icon} /></svg>
				</span>
				<span class="min-w-0">
					<span class="block font-display text-2xl font-extrabold leading-none tabular-nums">{s.n}</span>
					<span class="muted mt-1 block truncate text-[11px] font-semibold">{s.label}</span>
				</span>
			</div>
		{/each}
	</section>

	<!-- filter status -->
	{#if statuses.length > 1}
		<div class="flex flex-wrap items-center gap-2" role="group" aria-label="Filter status">
			<button
				type="button"
				aria-pressed={filter === 'all'}
				onclick={() => (filter = 'all')}
				class="rounded-full border px-3.5 py-1.5 text-xs font-bold transition aria-pressed:border-merah aria-pressed:bg-merah aria-pressed:text-white border-ink/15 hover:bg-ink/5"
			>
				{tx('pf.tn.all', 'Semua')} <span class="opacity-70">{all.length}</span>
			</button>
			{#each statuses as s (s)}
				<button
					type="button"
					aria-pressed={filter === s}
					onclick={() => (filter = s)}
					class="inline-flex items-center gap-1.5 rounded-full border border-ink/15 px-3.5 py-1.5 text-xs font-bold transition hover:bg-ink/5 aria-pressed:border-merah aria-pressed:bg-merah aria-pressed:text-white"
				>
					<span class="size-1.5 rounded-full {tone(s).dot}"></span>
					{stLabel(s)} <span class="opacity-70">{counts[s]}</span>
				</button>
			{/each}
		</div>
	{/if}

	<!-- daftar tenant -->
	<section class="grid grid-cols-1 gap-4 xl:grid-cols-2">
		{#each shown as m (m.id)}
			{@const tn = tone(m.onboarding_status)}
			<article class="card overflow-hidden transition hover:shadow-md">
				<div class="h-1 bg-linear-to-r {tn.ring}"></div>
				<div class="p-5">
					<div class="flex items-start gap-3.5">
						<span class="grid size-12 shrink-0 place-items-center rounded-2xl bg-linear-to-br {tn.ring} text-sm font-extrabold text-white shadow-sm">{inisial(m.display_name)}</span>
						<div class="min-w-0 flex-1">
							<div class="flex flex-wrap items-center gap-2">
								<h2 class="truncate text-[15px] font-extrabold">{m.display_name}</h2>
								<span class="pill {tn.pill}">{stLabel(m.onboarding_status)}</span>
							</div>
							<p class="muted mt-0.5 truncate text-xs">{m.legal_name}</p>
						</div>
					</div>

					<dl class="mt-4 grid grid-cols-2 gap-3 rounded-xl bg-ink/[.04] p-3 text-xs">
						<div>
							<dt class="muted text-[10px] font-bold uppercase tracking-wider">{tx('pf.tn.registered', 'Terdaftar')}</dt>
							<dd class="mt-0.5 font-semibold">{fmt(m.created_at)}</dd>
						</div>
						<div class="min-w-0">
							<dt class="muted text-[10px] font-bold uppercase tracking-wider">ID</dt>
							<dd class="mt-0.5 flex items-center gap-1.5">
								<code class="truncate font-mono text-[11px]">{m.id}</code>
								<button
									type="button"
									onclick={() => copy(m.id)}
									class="shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-bold text-merah transition hover:bg-merah/10"
									aria-label={tx('pf.tn.copy', 'Salin ID')}
								>
									{copied === m.id ? tx('pf.tn.copied', 'Tersalin ✓') : tx('pf.tn.copyShort', 'Salin')}
								</button>
							</dd>
						</div>
					</dl>

					<div class="mt-4 flex flex-wrap gap-2">
						<button
							type="button"
							onclick={() => (open = open === m.id ? null : m.id)}
							aria-expanded={open === m.id}
							class="inline-flex items-center gap-1.5 rounded-full bg-merah px-4 py-2 text-xs font-bold text-white transition hover:bg-merah-tua"
						>
							<svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5" /></svg>
							{t('pf.tn.enter')}
						</button>
						<button
							type="button"
							onclick={() => (act = act === m.id ? null : m.id)}
							aria-expanded={act === m.id}
							class="rounded-full border border-ink/25 px-4 py-2 text-xs font-bold transition hover:bg-ink hover:text-surface"
						>
							{t('pf.tn.activate')}
						</button>
					</div>

					{#if act === m.id || (form?.merchantId === m.id && (form?.activateError || form?.activated))}
						<form method="POST" action="?/activate" use:enhance autocomplete="off" class="mt-4 space-y-2.5 rounded-xl border border-ink/10 bg-ink/[.03] p-4">
							<input type="hidden" name="merchant_id" value={m.id} />
							<p class="muted text-xs leading-relaxed">{t('pf.tn.activateHint')}</p>
							<div class="flex flex-wrap items-start gap-2">
								<input name="password" type="password" minlength="12" maxlength="128" autocomplete="new-password" placeholder={t('pf.tn.pwPh')} class="inp min-w-56 flex-1 py-2" />
								<input name="note" required minlength="3" maxlength="500" placeholder={t('pf.tn.notePh')} class="inp min-w-56 flex-1 py-2" />
							</div>
							<div class="flex flex-wrap gap-2">
								<button name="mode" value="password" class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.tn.setPw')}</button>
								<button name="mode" value="verify" formnovalidate class="rounded-full border border-ink/30 px-4 py-2 text-xs font-bold">{t('pf.tn.verifyOnly')}</button>
							</div>
							{#if form?.merchantId === m.id && form.activateError}
								<p class="text-xs font-semibold text-red-700" role="alert">{form.activateError}</p>
							{:else if form?.merchantId === m.id && form.activated}
								<p class="text-xs font-semibold text-emerald-700">{t('pf.tn.done')}</p>
							{/if}
						</form>
					{/if}
					{#if open === m.id || form?.merchantId === m.id}
						<form method="POST" action="?/impersonate" use:enhance class="mt-4 space-y-2.5 rounded-xl border border-merah/20 bg-merah/[.04] p-4">
							<input type="hidden" name="merchant_id" value={m.id} />
							<textarea
								name="reason"
								required
								minlength="10"
								maxlength="500"
								rows="2"
								placeholder={t('pf.tn.reasonPh')}
								class="inp">{form?.merchantId === m.id ? form.reason : ''}</textarea>
							<div class="flex flex-wrap items-center gap-3">
								<button class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.tn.submit')}</button>
								{#if form?.merchantId === m.id && form.message}
									<p class="text-xs font-semibold text-red-700" role="alert">{form.message}</p>
								{/if}
							</div>
						</form>
					{/if}
				</div>
			</article>
		{:else}
			<div class="card col-span-full grid place-items-center gap-2 p-12 text-center">
				<span class="grid size-14 place-items-center rounded-full bg-ink/5">
					<svg viewBox="0 0 24 24" class="size-6 opacity-50" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
				</span>
				<p class="muted text-sm">{t('pf.tn.none')}</p>
			</div>
		{/each}
	</section>
	{#if all.length >= 50}
		<p class="muted text-center text-[11px]">{tx('pf.tn.limit', 'Menampilkan maksimal 50 tenant teratas. Persempit dengan pencarian.')}</p>
	{/if}
</div>
