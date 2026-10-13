<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { enhance } from '$app/forms';
	import { onMount } from 'svelte';
	import StateBadge from '../../../../lib/components/StateBadge.svelte';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import StatusBadge from '../../../../lib/components/StatusBadge.svelte';
	import { formatIDR } from '../../../../lib/utils/money';

	let { data, form } = $props();
	const p = $derived(data.payment);
	// Sisa yang masih bisa di-refund = nominal − refund selesai − refund yang sedang diproses.
	const refundable = $derived(p.amount - (p.refunded_amount ?? 0) - (p.refunding_amount ?? 0));
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
			{#if p.settlement_status}<dt class="muted">{t('trx.f.settlement')}</dt><dd><StateBadge kind="settle" value={p.settlement_status} /></dd>{/if}
			{#if p.refunded_amount}<dt class="muted">{t('trx.f.refunded')}</dt><dd class="font-bold">{formatIDR(p.refunded_amount)}</dd>{/if}
			{#if p.refunding_amount}<dt class="muted">{t('trx.f.refunding')}</dt><dd class="font-bold">{formatIDR(p.refunding_amount)}</dd>{/if}
		</dl>
		{#if p.description}<p class="muted mt-4 text-[11px]">{p.description}</p>{/if}
	</section>
</div>

<!--
	Riwayat perpindahan status. Read-only: halaman ini hanya menampilkan apa yang
	sistem terima. Baris `applied=false` adalah laporan gateway yang bertentangan
	dengan status final; ditampilkan sebagai bukti rekonsiliasi (PRD §0 #2).
	Status tetap berlabel teks, tidak hanya mengandalkan warna (PRD §4.11).
-->
{#if data.timeline.length > 0}
	<section class="card mt-5 p-6">
		<h2 class="mb-1 text-sm font-bold">{t('trx.timeline')}</h2>
		<p class="muted mb-4 text-[11px]">{t('trx.timelineNote')}</p>
		<ol class="space-y-3">
			{#each data.timeline as ev, i (i)}
				<li class="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs">
					<span class="muted w-40 shrink-0 tabular-nums">{fmt(ev.created_at)}</span>
					<span class="font-bold">
						{ev.from_status} &rarr; {ev.to_status}
					</span>
					<span class="muted">{t(`trx.tl.source.${ev.source}`)}</span>
					{#if ev.raw_gateway_status}
						<span class="muted font-mono text-[11px]">{t('trx.tl.raw', { code: ev.raw_gateway_status })}</span>
					{/if}
					{#if ev.applied}
						<span class="pill bg-emerald-100 text-emerald-800">{t('trx.tl.applied')}</span>
					{:else}
						<span class="pill bg-rose-100 text-rose-800">{t('trx.tl.notApplied')}</span>
					{/if}
				</li>
			{/each}
		</ol>
	</section>
{/if}
{#if data.canRefund && p.status === 'succeeded' && refundable > 0}
	<form method="POST" action="?/refund" class="card mt-5 max-w-md space-y-4 p-6" use:enhance>
		<h2 class="text-sm font-bold">{t('rf.request')}</h2>
		<input type="hidden" name="idem" value={data.idem} />
		<div>
			<label class="lbl mb-1.5" for="amount">{t('po.amount')} <span class="muted font-normal">· {t('rf.max', { max: formatIDR(refundable) })}</span></label>
			<input id="amount" class="inp" name="amount" inputmode="numeric" pattern="[0-9.]*" required />
		</div>
		<div>
			<label class="lbl mb-1.5" for="reason">{t('rf.reason')}</label>
			<input id="reason" class="inp" name="reason" minlength="3" maxlength="255" placeholder={t('rf.reasonPh')} required />
		</div>
		{#if form?.message}<p class="text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
		{#if form?.done}<p class="text-xs font-semibold text-emerald-700" role="status"><a href="/dashboard/refunds" class="underline">{t('nav.refunds')}</a></p>{/if}
		<button type="submit" class="btn btn-light">{t('rf.requestBtn')}</button>
	</form>
{/if}
<p class="mt-5"><a href="/dashboard/transactions/new" class="text-xs font-bold text-merah hover:underline">{t('trx.another')}</a></p>

<style>
	.qr :global(svg) { width: 100%; height: auto; display: block; }
</style>
