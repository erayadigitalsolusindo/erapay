<script lang="ts">
	import { getI18n } from '../../lib/i18n/i18n.svelte';
	import StatusBadge from '../../lib/components/StatusBadge.svelte';
	import { formatIDR } from '../../lib/utils/money';

	let { data } = $props();
	const i18n = getI18n();
	const { t } = i18n;

	const s = $derived(data.platform ? null : data.summary);
	const cards = $derived(
		s
			? ([
					{ label: t('ov.gross'), value: s.totals.gross, hint: t('ov.gross.hint', { n: s.totals.succeeded }), main: true },
					{ label: t('ov.net'), value: s.totals.merchant_net, hint: t('ov.net.hint') },
					{ label: t('ov.fees'), value: s.totals.mdr_fee + s.totals.platform_fee, hint: t('ov.fees.hint', { mdr: formatIDR(s.totals.mdr_fee), fee: formatIDR(s.totals.platform_fee) }) },
					{ label: t('ov.refunded'), value: s.totals.refunded, hint: t('ov.refunded.hint') }
				] as const)
			: []
	);

	// Grafik batang: tinggi proporsional terhadap hari terbesar; tiap batang punya <title> sebagai teks alternatif.
	const W = 600;
	const H = 140;
	const max = $derived(Math.max(1, ...(s?.daily.map((d) => d.gross) ?? [0])));
	const step = $derived(s && s.daily.length ? W / s.daily.length : W);
	const dayLabel = (iso: string) => new Date(`${iso}T00:00:00`).toLocaleDateString(i18n.locale, { day: 'numeric', month: 'short' });

	const mix = $derived(
		s
			? ([
					['succeeded', s.totals.succeeded],
					['pending', s.totals.pending],
					['failed', s.totals.failed],
					['expired', s.totals.expired],
					['cancelled', s.totals.cancelled],
					['unknown', s.totals.unknown]
				] as const).filter(([, n]) => n > 0)
			: []
	);
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Jakarta' });
</script>

{#if data.platform}
	<div class="card p-6">
		<h2 class="text-sm font-bold">{t('overview.title')}</h2>
		<p class="muted mt-2 text-xs">{t('ov.platform')}</p>
	</div>
{:else}
	<div class="mb-5 flex flex-wrap items-center justify-between gap-3">
		<h2 class="text-sm font-bold">{t('overview.title')}</h2>
		<nav class="flex flex-wrap items-center gap-1.5" aria-label={t('ov.period')}>
			{#each data.periods as n (n)}
				<a
					href="?days={n}"
					aria-current={data.days === n ? 'true' : undefined}
					class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white"
					>{t('ov.days', { n })}</a
				>
			{/each}
		</nav>
	</div>

	{#if data.canReport && data.summary === null}
		<p class="mb-5 text-xs font-semibold text-red-700" role="alert">{t('ov.loadFail')}</p>
	{/if}

	{#if s}
		<section class="mb-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label={t('overview.title')}>
			{#each cards as c (c.label)}
				<div class="card p-4 {'main' in c && c.main ? 'border-emas' : ''}">
					<div class="muted text-[11px] font-bold uppercase">{c.label}</div>
					<div class="mt-1 font-display text-xl font-extrabold">{formatIDR(c.value)}</div>
					<div class="muted mt-1 text-[11px]">{c.hint}</div>
				</div>
			{/each}
		</section>

		{#if data.balance}
			<a href="/dashboard/payouts" class="card mb-6 flex flex-wrap items-center justify-between gap-2 p-4 hover:bg-ink/5">
				<span>
					<span class="muted text-[11px] font-bold uppercase">{t('ov.balance')}</span>
					<span class="mt-1 block font-display text-xl font-extrabold">{formatIDR(data.balance.payable)}</span>
				</span>
				<span class="muted text-[11px]">{t('ov.balance.hint', { a: formatIDR(data.balance.available), p: formatIDR(data.balance.pending) })}</span>
			</a>
		{/if}

		<div class="mb-6 grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
			<section class="card p-5">
				<h3 class="text-sm font-bold">{t('ov.chart')}</h3>
				<p class="muted mb-3 text-[11px]">{t('ov.chartNote')}</p>
				{#if s.totals.succeeded === 0}
					<p class="muted py-10 text-center text-xs">{t('ov.noSales')}</p>
				{:else}
					<svg viewBox="0 0 {W} {H + 18}" class="h-auto w-full" role="img" aria-label={t('ov.chart')}>
						<line x1="0" y1={H} x2={W} y2={H} stroke="currentColor" stroke-opacity="0.15" />
						{#each s.daily as d, i (d.date)}
							{@const h = d.gross > 0 ? Math.max(3, (d.gross / max) * (H - 8)) : 0}
							<rect x={i * step + step * 0.12} y={H - h} width={step * 0.76} height={h} rx="2" class="fill-merah" fill-opacity={d.gross > 0 ? 0.85 : 0}>
								<title>{t('ov.day', { date: dayLabel(d.date), n: d.count, amount: formatIDR(d.gross) })}</title>
							</rect>
						{/each}
						<text x="0" y={H + 13} font-size="10" fill="currentColor" fill-opacity="0.6">{dayLabel(s.daily[0].date)}</text>
						<text x={W} y={H + 13} font-size="10" text-anchor="end" fill="currentColor" fill-opacity="0.6">{dayLabel(s.daily[s.daily.length - 1].date)}</text>
					</svg>
				{/if}
			</section>

			<section class="card p-5">
				<h3 class="mb-3 text-sm font-bold">{t('ov.statusMix')}</h3>
				{#if mix.length === 0}
					<p class="muted text-xs">{t('ov.noSales')}</p>
				{:else}
					<ul class="space-y-2.5 text-xs">
						{#each mix as [status, n] (status)}
							<li>
								<div class="mb-1 flex items-center justify-between gap-2">
									<StatusBadge {status} />
									<span class="font-bold">{n}</span>
								</div>
								<div class="h-1.5 overflow-hidden rounded-full bg-ink/10" aria-hidden="true">
									<div class="h-full rounded-full bg-merah" style="width: {Math.round((n / s.totals.payments) * 100)}%"></div>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		</div>
	{/if}

	{#if data.canPayments}
		<section class="card overflow-hidden">
			<div class="flex items-center justify-between gap-3 p-5">
				<h3 class="text-sm font-bold">{t('ov.recent')}</h3>
				<a href="/dashboard/transactions" class="text-xs font-bold text-merah hover:underline">{t('ov.viewAll')}</a>
			</div>
			{#if data.recent === null}
				<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('trx.loadFail')}</p>
			{:else if data.recent.length === 0}
				<p class="muted px-5 pb-5 text-xs">{t('trx.listEmpty')}</p>
			{:else}
				<div class="overflow-x-auto">
					<table>
						<thead><tr><th>{t('c.created')}</th><th>{t('trx.ref')}</th><th>{t('c.amount')}</th><th>{t('c.status')}</th></tr></thead>
						<tbody>
							{#each data.recent as p (p.id)}
								<tr>
									<td class="muted">{fmt(p.created_at)}</td>
									<td><a class="font-bold text-merah hover:underline" href="/dashboard/transactions/{p.id}">{p.merchant_reference}</a></td>
									<td class="font-bold">{formatIDR(p.amount)}</td>
									<td><StatusBadge status={p.status} /></td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>
	{/if}
{/if}
