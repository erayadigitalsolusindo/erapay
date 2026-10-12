<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { onMount } from 'svelte';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import StatusBadge from '../../../../lib/components/StatusBadge.svelte';
	import { formatIDR } from '../../../../lib/utils/money';

	let { data } = $props();
	const p = $derived(data.payment);
	const fmt = (iso: string | null) => (iso ? new Date(iso).toLocaleString(i18n.locale) : '-');

	const i18n = getI18n();
	const { t } = i18n;

	// Status hanya dibaca dari server; halaman tidak pernah menandai lunas sendiri.
	onMount(() => {
		const t = setInterval(() => {
			if (p.status === 'pending' || p.status === 'unknown' || p.status === 'created') invalidateAll();
		}, 5000);
		return () => clearInterval(t);
	});
</script>

<div class="mb-5 flex flex-wrap items-center gap-3"><h2 class="font-bold">{t('trx.payment', { ref: p.merchant_reference })}</h2><StatusBadge status={p.status} /></div>

<div class="flex flex-wrap items-start gap-4">
	{#if data.qr}
		<section class="card p-6 text-center">
			<!-- eslint-disable-next-line svelte/no-at-html-tags -- SVG dibuat server dari library QR, bukan input pengguna -->
			<div class="qr mx-auto w-64 max-w-full" aria-label={t('trx.qrLabel')}>{@html data.qr}</div>
			<p class="mt-3 text-xs">{t('trx.scan')}</p>
			<p class="muted mt-1 text-[11px]">{t('trx.validUntil', { date: fmt(p.expires_at) })}</p>
		</section>
	{:else if p.status === 'pending'}
		<section class="card p-6 text-xs">{t('trx.qrMissing')}</section>
	{/if}

	<section class="card min-w-72 p-6">
		<dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-xs">
			<dt class="muted">{t('trx.f.amount')}</dt><dd class="font-bold">{formatIDR(p.amount)}</dd>
			<dt class="muted">{t('trx.f.method')}</dt><dd class="font-bold">{p.payment_method} · {p.gateway}</dd>
			<dt class="muted">{t('trx.f.created')}</dt><dd class="font-bold">{fmt(p.created_at)}</dd>
			<dt class="muted">{t('trx.f.final')}</dt><dd class="font-bold">{fmt(p.final_at)}</dd>
			<dt class="muted">{t('trx.f.mdr')}</dt><dd class="font-bold">{formatIDR(p.fee.mdr_fee)}</dd>
			<dt class="muted">{t('trx.f.platformFee')}</dt><dd class="font-bold">{formatIDR(p.fee.platform_fee)}</dd>
			<dt class="muted">{t('trx.f.net')}</dt><dd class="font-bold">{formatIDR(p.fee.merchant_net)}</dd>
		</dl>
		{#if p.description}<p class="muted mt-4 text-[11px]">{p.description}</p>{/if}
	</section>
</div>
<p class="mt-5"><a href="/dashboard/transactions/new" class="text-xs font-bold text-merah hover:underline">{t('trx.another')}</a></p>

<style>
	.qr :global(svg) { width: 100%; height: auto; display: block; }
</style>
