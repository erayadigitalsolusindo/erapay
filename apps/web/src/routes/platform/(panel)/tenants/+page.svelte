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
	const TONE: Record<string, { pill: string; dot: string }> = {
		approved: { pill: 'bg-emerald-500/15 text-emerald-700', dot: 'bg-emerald-500' },
		submitted: { pill: 'bg-amber-500/15 text-amber-700', dot: 'bg-amber-500' },
		under_review: { pill: 'bg-sky-500/15 text-sky-700', dot: 'bg-sky-500' },
		action_required: { pill: 'bg-orange-500/15 text-orange-700', dot: 'bg-orange-500' },
		rejected: { pill: 'bg-red-500/15 text-red-700', dot: 'bg-red-500' },
		suspended: { pill: 'bg-red-500/15 text-red-700', dot: 'bg-red-500' },
		draft: { pill: 'bg-ink/10 text-ink/70', dot: 'bg-ink/40' }
	};
	const tone = (s: string) => TONE[s] ?? TONE.draft;

	type M = { own?: boolean; id: string; display_name: string; legal_name: string; onboarding_status: string; created_at: string };
	const all = $derived(data.merchants as M[]);
	const counts = $derived(
		all.reduce<Record<string, number>>((a, m) => ((a[m.onboarding_status] = (a[m.onboarding_status] ?? 0) + 1), a), {})
	);
	const pending = $derived((counts.submitted ?? 0) + (counts.under_review ?? 0) + (counts.action_required ?? 0));
	const blocked = $derived((counts.rejected ?? 0) + (counts.suspended ?? 0));
	const GROUPS: Record<string, string[]> = { approved: ['approved'], pending: ['submitted', 'under_review', 'action_required'], blocked: ['rejected', 'suspended'] };
	const shown = $derived(filter === 'all' ? all : all.filter((m) => GROUPS[filter]?.includes(m.onboarding_status)));

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

<div class="space-y-5">
	<header class="flex flex-wrap items-end justify-between gap-4">
		<div class="max-w-2xl">
			<h1 class="font-display text-2xl font-extrabold">{t('pf.nav.tenants')}</h1>
			<p class="muted mt-1 text-[13px] leading-relaxed">
				{t('pf.tn.sub')}
				<a href="/platform/audit" class="font-bold text-merah underline underline-offset-2">{t('pf.tn.subLink')}</a>
			</p>
		</div>
		<form method="GET" class="flex w-full gap-2 sm:w-auto">
			<div class="relative min-w-0 flex-1 sm:w-72">
				<svg viewBox="0 0 24 24" class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 opacity-40" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
				<input name="q" value={data.q} placeholder={t('pf.tn.searchPh')} class="inp py-2 pl-9" />
			</div>
			<button class="rounded-xl bg-merah px-4 py-2 text-xs font-bold text-white transition hover:bg-merah-tua">{t('pf.tn.search')}</button>
		</form>
	</header>

	{#if data.error}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>{/if}

	<!-- ringkasan sekaligus filter -->
	<div class="grid grid-cols-2 gap-3 lg:grid-cols-4" role="group" aria-label="Filter status">
		{#each [
			{ key: 'all', label: tx('pf.tn.stTotal', 'Total tenant'), n: all.length },
			{ key: 'approved', label: stLabel('approved'), n: counts.approved ?? 0 },
			{ key: 'pending', label: tx('pf.tn.stPending', 'Dalam proses'), n: pending },
			{ key: 'blocked', label: tx('pf.tn.stBlocked', 'Ditolak / ditangguhkan'), n: blocked }
		] as s (s.key)}
			<button
				type="button"
				aria-pressed={filter === s.key}
				onclick={() => (filter = s.key)}
				class="card px-4 py-3 text-left transition hover:shadow-md aria-pressed:border-merah aria-pressed:ring-2 aria-pressed:ring-merah/15"
			>
				<span class="block font-display text-2xl font-extrabold leading-none tabular-nums">{s.n}</span>
				<span class="muted mt-1.5 block truncate text-[11px] font-semibold">{s.label}</span>
			</button>
		{/each}
	</div>

	<section class="card overflow-hidden">
		<div class="overflow-x-auto">
			<table class="w-full min-w-[820px] text-left text-sm">
				<thead class="bg-gading text-[10px] font-bold uppercase tracking-wider text-merah-gelap/60">
					<tr>
						<th class="px-4 py-3">Tenant</th>
						<th class="px-4 py-3">Status</th>
						<th class="px-4 py-3">{tx('pf.tn.registered', 'Terdaftar')}</th>
						<th class="px-4 py-3">ID</th>
						<th class="px-4 py-3 text-right">{tx('pf.tn.actions', 'Aksi')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-merah-gelap/10">
					{#each shown as m (m.id)}
						{@const tn = tone(m.onboarding_status)}
						{@const expanded = open === m.id || act === m.id || form?.merchantId === m.id}
						<tr class="align-middle hover:bg-gading/60">
							<td class="px-4 py-3">
								<div class="flex items-center gap-3">
									<span class="grid size-9 shrink-0 place-items-center rounded-full bg-merah-gelap/10 text-xs font-extrabold text-merah-gelap">{inisial(m.display_name)}</span>
									<div class="min-w-0">
										<p class="truncate text-[13px] font-bold">{m.display_name}</p>
										<p class="muted truncate text-[11px]">{m.legal_name}</p>
									</div>
								</div>
							</td>
							<td class="px-4 py-3"><span class="pill {tn.pill}">{stLabel(m.onboarding_status)}</span></td>
							<td class="whitespace-nowrap px-4 py-3 text-xs font-semibold">{fmt(m.created_at)}</td>
							<td class="px-4 py-3">
								<button
									type="button"
									onclick={() => copy(m.id)}
									title={m.id}
									aria-label={tx('pf.tn.copy', 'Salin ID')}
									class="rounded-md px-1.5 py-0.5 font-mono text-[11px] transition hover:bg-merah/10"
								>
									{copied === m.id ? tx('pf.tn.copied', 'Tersalin ✓') : m.id.slice(0, 8) + '…'}
								</button>
							</td>
							<td class="px-4 py-3">
								<div class="flex items-center justify-end gap-1.5">
									{#if m.own}
										<form method="POST" action="?/impersonate">
											<input type="hidden" name="merchant_id" value={m.id} />
											<input type="hidden" name="own" value="1" />
											<button class="rounded-lg bg-merah px-3 py-1.5 text-xs font-bold text-white transition hover:bg-merah-tua">{t('pf.tn.enter')}</button>
										</form>
									{:else}
										<button
											type="button"
											onclick={() => ((open = open === m.id ? null : m.id), (act = null))}
											aria-expanded={open === m.id}
											class="rounded-lg bg-merah px-3 py-1.5 text-xs font-bold text-white transition hover:bg-merah-tua"
										>{t('pf.tn.enter')}</button>
									{/if}
									<a href="/platform/tenants/{m.id}/gateway?n={encodeURIComponent(m.display_name)}" class="rounded-lg border border-merah-gelap/20 px-3 py-1.5 text-xs font-bold transition hover:bg-merah-gelap/5">{tx('pf.tn.gateway', 'Gateway')}</a>
									<button
										type="button"
										onclick={() => ((act = act === m.id ? null : m.id), (open = null))}
										aria-expanded={act === m.id}
										class="rounded-lg border border-merah-gelap/20 px-3 py-1.5 text-xs font-bold transition hover:bg-merah-gelap/5"
									>{t('pf.tn.activate')}</button>
									<form
										method="POST"
										action="?/remove"
										use:enhance={({ cancel, formData }) => {
											const typed = prompt(`Hapus PERMANEN tenant "${m.display_name}" beserta SEMUA datanya (gateway, API key, user pemilik, transaksi, payout, refund, ledger)? Tidak bisa dibatalkan.\n\nKetik nama tenant untuk melanjutkan:`);
											if (typed === null) return cancel();
											formData.set('confirm', typed.trim());
										}}
									>
										<input type="hidden" name="merchant_id" value={m.id} />
										<button class="rounded-lg px-3 py-1.5 text-xs font-bold text-red-700 transition hover:bg-red-600 hover:text-white">Hapus</button>
									</form>
								</div>
							</td>
						</tr>
						{#if expanded || (form?.merchantId === m.id && form.removeError)}
							<tr class="bg-gading/60">
								<td colspan="5" class="space-y-3 px-4 py-4">
									{#if form?.merchantId === m.id && form.removeError}
										<p class="text-xs font-semibold text-red-700" role="alert">{form.removeError}</p>
									{/if}
									{#if act === m.id || (form?.merchantId === m.id && (form?.activateError || form?.activated))}
										<form method="POST" action="?/activate" use:enhance autocomplete="off" class="max-w-3xl space-y-2.5">
											<input type="hidden" name="merchant_id" value={m.id} />
											<p class="muted text-xs leading-relaxed">{t('pf.tn.activateHint')}</p>
											<div class="flex flex-wrap items-start gap-2">
												<input name="password" type="password" minlength="12" maxlength="128" autocomplete="new-password" placeholder={t('pf.tn.pwPh')} class="inp min-w-56 flex-1 py-2" />
												<input name="note" required minlength="3" maxlength="500" placeholder={t('pf.tn.notePh')} class="inp min-w-56 flex-1 py-2" />
											</div>
											<div class="flex flex-wrap gap-2">
												<button name="mode" value="password" class="rounded-lg bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.tn.setPw')}</button>
												<button name="mode" value="verify" formnovalidate class="rounded-lg border border-merah-gelap/25 px-4 py-2 text-xs font-bold">{t('pf.tn.verifyOnly')}</button>
											</div>
											{#if form?.merchantId === m.id && form.activateError}
												<p class="text-xs font-semibold text-red-700" role="alert">{form.activateError}</p>
											{:else if form?.merchantId === m.id && form.activated}
												<p class="text-xs font-semibold text-emerald-700">{t('pf.tn.done')}</p>
											{/if}
										</form>
									{/if}
									{#if open === m.id || (form?.merchantId === m.id && form.message)}
										<form method="POST" action="?/impersonate" use:enhance class="max-w-3xl space-y-2.5">
											<input type="hidden" name="merchant_id" value={m.id} />
											<textarea name="reason" required minlength="10" maxlength="500" rows="2" placeholder={t('pf.tn.reasonPh')} class="inp">{form?.merchantId === m.id ? form.reason : ''}</textarea>
											<div class="flex flex-wrap items-center gap-3">
												<button class="rounded-lg bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.tn.submit')}</button>
												{#if form?.merchantId === m.id && form.message}
													<p class="text-xs font-semibold text-red-700" role="alert">{form.message}</p>
												{/if}
											</div>
										</form>
									{/if}
								</td>
							</tr>
						{/if}
					{:else}
						<tr><td colspan="5" class="muted px-4 py-12 text-center text-sm">{t('pf.tn.none')}</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
	{#if all.length >= 50}
		<p class="muted text-center text-[11px]">{tx('pf.tn.limit', 'Menampilkan maksimal 50 tenant teratas. Persempit dengan pencarian.')}</p>
	{/if}
</div>
