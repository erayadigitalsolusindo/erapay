<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { onMount } from 'svelte';
	import StatusBadge from '../../../../lib/components/StatusBadge.svelte';
	import { formatIDR } from '../../../../lib/utils/money';

	let { data } = $props();
	const p = $derived(data.payment);
	const fmt = (iso: string | null) => (iso ? new Date(iso).toLocaleString('id-ID') : '-');

	// Status hanya dibaca dari server; halaman tidak pernah menandai lunas sendiri.
	onMount(() => {
		const t = setInterval(() => {
			if (p.status === 'pending' || p.status === 'unknown' || p.status === 'created') invalidateAll();
		}, 5000);
		return () => clearInterval(t);
	});
</script>

<h1>Pembayaran {p.merchant_reference}</h1>
<p><StatusBadge status={p.status} /></p>

<div class="grid">
	{#if data.qr}
		<section class="card qr">
			<!-- eslint-disable-next-line svelte/no-at-html-tags -- SVG dibuat server dari library QR, bukan input pengguna -->
			<div class="svg" aria-label="Kode QRIS">{@html data.qr}</div>
			<p>Pindai dengan aplikasi pembayaran QRIS.</p>
			<p><small>Berlaku sampai {fmt(p.expires_at)}</small></p>
		</section>
	{:else if p.status === 'pending'}
		<section class="card"><p>Kode QR tidak tersedia.</p></section>
	{/if}

	<section class="card">
		<dl>
			<dt>Nominal</dt><dd>{formatIDR(p.amount)}</dd>
			<dt>Metode</dt><dd>{p.payment_method} · {p.gateway}</dd>
			<dt>Dibuat</dt><dd>{fmt(p.created_at)}</dd>
			<dt>Selesai</dt><dd>{fmt(p.final_at)}</dd>
			<dt>MDR</dt><dd>{formatIDR(p.fee.mdr_fee)}</dd>
			<dt>Biaya jasa EraPay</dt><dd>{formatIDR(p.fee.platform_fee)}</dd>
			<dt>Diterima merchant</dt><dd>{formatIDR(p.fee.merchant_net)}</dd>
		</dl>
		{#if p.description}<p><small>{p.description}</small></p>{/if}
	</section>
</div>
<p><a href="/dashboard/transactions/new">Buat QRIS lain</a></p>

<style>
	.grid { display: flex; flex-wrap: wrap; gap: 1rem; align-items: flex-start; }
	.card { padding: 1.25rem; background: #fff; border: 1px solid #e5e7eb; border-radius: 1rem; }
	.qr { text-align: center; }
	.svg { width: 16rem; max-width: 100%; margin: 0 auto; }
	.svg :global(svg) { width: 100%; height: auto; display: block; }
	dl { display: grid; grid-template-columns: auto 1fr; gap: 0.35rem 1rem; margin: 0; }
	dt { color: #6b7280; }
	dd { margin: 0; font-weight: 600; }
	a { color: #c8102e; }
</style>
