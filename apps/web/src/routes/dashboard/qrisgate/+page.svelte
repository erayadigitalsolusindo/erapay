<script lang="ts">
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { Key } from '../../../lib/i18n/messages';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import { formatIDR } from '../../../lib/utils/money';
	let { data, form } = $props();
	const { t } = getI18n();
	// Top-up QRIS yang masih menunggu bayar: segarkan tiap 5 detik agar saldo naik begitu pembayaran masuk.
	const waiting = $derived(data.topups.some((x: { method: string; status: string }) => x.method === 'qris' && x.status === 'requested'));
	$effect(() => {
		if (!waiting) return;
		const id = setInterval(() => void invalidateAll(), 5000);
		return () => clearInterval(id);
	});
	const when = (iso: string) => new Date(iso).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
</script>

<h1 class="text-xl font-extrabold">{t('qg.title')}</h1>
<p class="muted mt-1 mb-4 max-w-2xl text-xs">{t('qg.sub')}</p>

<section class="card mb-4 p-5" role="status">
	<p class="text-sm font-bold">{t('qg.status')}</p>
	<p class="muted mt-1 text-xs">{t('qg.statusNote')}</p>
	{#if data.overview}
		<p class="mt-2 text-xs font-semibold">
			{data.overview.fee > 0 ? t('qg.fee.some').replace('{amount}', formatIDR(data.overview.fee)) : t('qg.fee.zero')}
		</p>
	{/if}
</section>

{#if data.overview}
	<section class="card mb-4 p-5">
		<p class="muted text-xs font-semibold">{t('qg.bal.title')}</p>
		<p class="mt-1 text-2xl font-extrabold {data.overview.balance < 0 ? 'text-red-700' : ''}">{formatIDR(data.overview.balance)}</p>
		{#if data.overview.balance < 0}<p class="muted mt-1 text-xs">{t('qg.bal.neg')}</p>{/if}
	</section>

	<section class="card mb-4 p-5">
		<h2 class="text-sm font-bold">{t('qg.topup.title')}</h2>
		<p class="muted mt-1 mb-3 max-w-2xl text-xs">{t('qg.topup.help')}</p>
		{#if data.canRequest}
			<form method="POST" action="?/topup" use:enhance class="flex flex-wrap items-end gap-3">
				<input type="hidden" name="idem" value={data.idem} />
				<div>
					<label class="lbl mb-1" for="qg-amount">{t('qg.topup.amount')}</label>
					<input id="qg-amount" name="amount" inputmode="numeric" pattern="[0-9.]*" required class="inp" />
				</div>
				<button class="rounded-full bg-merah px-5 py-2 text-xs font-bold text-white">{t('qg.topup.submit')}</button>
			</form>
			<form method="POST" action="?/topupQris" use:enhance class="mt-3 flex flex-wrap items-end gap-3">
				<input type="hidden" name="idem" value={data.idem + '-q'} />
				<div>
					<label class="lbl mb-1" for="qg-amount-qris">{t('qg.topup.amount')}</label>
					<input id="qg-amount-qris" name="amount" inputmode="numeric" pattern="[0-9.]*" required class="inp" />
				</div>
				<button class="rounded-full bg-emas px-5 py-2 text-xs font-bold text-ink">{t('qg.topup.qris')}</button>
			</form>
			<p class="muted mt-1 text-[11px]">{t('qg.topup.qrisHelp')}</p>
			{#if (form as { unavailable?: boolean } | null)?.unavailable}<p class="mt-2 text-xs font-semibold text-red-700" role="alert">{t('qg.qris.unavailable')}</p>
			{:else if form?.error}<p class="mt-2 text-xs font-semibold text-red-700" role="alert">{t('qg.err')}</p>{/if}
			{#if form?.sentQris}<p class="mt-2 text-xs font-semibold text-emerald-700" role="status">{t('qg.sentQris')}</p>{/if}
			{#if form?.sent}<p class="mt-2 text-xs font-semibold text-emerald-700" role="status">{t('qg.sent')}</p>{/if}
		{/if}
	</section>

	<section class="card mb-4 overflow-x-auto p-5">
		<h2 class="mb-2 text-sm font-bold">{t('qg.topup.list')}</h2>
		{#if data.topups.length === 0}
			<p class="muted text-xs">{t('qg.empty')}</p>
		{:else}
			<table class="w-full text-left text-xs">
				<thead><tr><th class="py-1">{t('qg.col.date')}</th><th>{t('qg.col.amount')}</th><th>{t('qg.col.status')}</th><th></th></tr></thead>
				<tbody>
					{#each data.topups as tp (tp.id)}
						<tr class="border-t border-ink/10">
							<td class="py-1.5">{when(tp.created_at)}</td>
							<td>{formatIDR(tp.amount)}</td>
							<td>
								{t(`qg.st.${tp.status}` as Key)}{tp.method === 'qris' ? ' · QRIS' : ''}
								{#if data.qr[tp.id]}
									<div class="mt-2 w-40 rounded-lg bg-white p-2" aria-label="QRIS">{@html data.qr[tp.id]}</div>
								{/if}
							</td>
							<td>
								{#if tp.status === 'requested' && data.canRequest}
									<form method="POST" action="?/cancel" use:enhance><input type="hidden" name="id" value={tp.id} /><button class="font-bold text-merah">{t('qg.cancel')}</button></form>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</section>

	<section class="card mb-4 overflow-x-auto p-5">
		<h2 class="mb-2 text-sm font-bold">{t('qg.hist.title')}</h2>
		{#if data.history.length === 0}
			<p class="muted text-xs">{t('qg.empty')}</p>
		{:else}
			<table class="w-full text-left text-xs">
				<thead><tr><th class="py-1">{t('qg.col.date')}</th><th></th><th>{t('qg.col.amount')}</th></tr></thead>
				<tbody>
					{#each data.history as m, i (i)}
						<tr class="border-t border-ink/10">
							<td class="py-1.5">{when(m.at)}</td>
							<td>{m.kind === 'qrisgate_topup' ? t('qg.hist.topup') : t('qg.hist.fee')}</td>
							<td class={m.direction === 'debit' ? 'text-red-700' : 'text-emerald-700'}>{m.direction === 'debit' ? '−' : '+'}{formatIDR(m.amount)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</section>
{/if}

<div class="grid gap-4 md:grid-cols-2">
	<section class="card p-5">
		<h2 class="text-sm font-bold">{t('qg.how.title')}</h2>
		<ol class="mt-2 list-decimal space-y-1 pl-5 text-xs">
			<li>{t('qg.how.1')}</li>
			<li>{t('qg.how.2')}</li>
			<li>{t('qg.how.3')}</li>
		</ol>
	</section>
	<section class="card p-5">
		<h2 class="text-sm font-bold">{t('qg.diff.title')}</h2>
		<ul class="mt-2 list-disc space-y-1 pl-5 text-xs">
			<li>{t('qg.diff.1')}</li>
			<li>{t('qg.diff.2')}</li>
			<li>{t('qg.diff.3')}</li>
		</ul>
	</section>
</div>
