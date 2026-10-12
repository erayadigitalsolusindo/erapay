<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import { formatIDR } from '../../../../lib/utils/money';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });

	let selected = $state<string[]>([]);
	// Hanya perkiraan untuk Finance; angka yang membukukan tetap hitungan server.
	const expected = $derived((data.unsettled ?? []).filter((u) => selected.includes(u.payment_id)).reduce((s, u) => s + u.expected_net, 0));
	const gateways = $derived([...new Set((data.unsettled ?? []).map((u) => u.gateway))]);
	const today = new Date().toISOString().slice(0, 10);
</script>

{#if form?.booked}
	<p class="mb-4 text-xs font-semibold text-emerald-700" role="status">{t('fin.booked', { n: form.booked.payment_count, net: formatIDR(form.booked.expected_net) })}</p>
{/if}
{#if form?.mismatch}
	<p class="mb-4 max-w-2xl rounded-xl border border-red-700/30 bg-red-500/10 p-3 text-xs font-semibold text-red-700" role="alert">
		{t('fin.mismatch', { reported: formatIDR(form.mismatch.reported), expected: formatIDR(form.mismatch.expected), diff: formatIDR(form.mismatch.diff) })}
	</p>
{/if}
{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<div class="mb-6 grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
	<section class="card overflow-hidden">
		<div class="flex flex-wrap items-center justify-between gap-3 p-5">
			<h2 class="text-sm font-bold">{t('fin.unsettled')}</h2>
			{#if gateways.length > 1 || data.gateway}
				<form method="GET" class="flex items-center gap-2">
					<label class="lbl" for="gw">{t('fin.gateway')}</label>
					<input id="gw" name="gateway" class="inp w-32 py-1.5" value={data.gateway} />
					<button class="btn btn-light" type="submit">OK</button>
				</form>
			{/if}
		</div>
		{#if data.unsettled === null}
			<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
		{:else if data.unsettled.length === 0}
			<p class="muted px-5 pb-5 text-xs">{t('fin.unsettledNone')}</p>
		{:else}
			<div class="overflow-x-auto">
				<table>
					<thead><tr><th>{t('fin.pick')}</th><th>{t('fin.col.payment')}</th><th>{t('fin.gateway')}</th><th>{t('fin.col.gross')}</th><th>{t('fin.col.mdr')}</th><th>{t('fin.col.net')}</th><th>{t('fin.col.final')}</th></tr></thead>
					<tbody>
						{#each data.unsettled as u (u.payment_id)}
							<tr>
								<td><input type="checkbox" class="accent-merah" form="batch" name="payment_ids" value={u.payment_id} bind:group={selected} aria-label={u.payment_id} /></td>
								<td><code>{u.gateway_reference || u.payment_id.slice(0, 8)}</code></td>
								<td>{u.gateway}</td>
								<td>{formatIDR(u.gross)}</td>
								<td>{formatIDR(u.mdr_fee)}</td>
								<td class="font-bold">{formatIDR(u.expected_net)}</td>
								<td class="muted">{fmt(u.final_at)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>

	{#if data.canManage}
		<form id="batch" method="POST" action="?/record" class="card h-fit space-y-4 p-6" use:enhance>
			<h2 class="text-sm font-bold">{t('fin.record')}</h2>
			<p class="muted text-xs">{t('fin.recordNote')}</p>
			<p class="text-xs font-bold">{t('fin.selected', { n: selected.length, net: formatIDR(expected) })}</p>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label class="lbl mb-1.5" for="b-gw">{t('fin.gateway')}</label>
					<input id="b-gw" class="inp" name="gateway" value={data.gateway || gateways[0] || ''} required />
				</div>
				<div>
					<label class="lbl mb-1.5" for="b-cur">{t('fin.currency')}</label>
					<input id="b-cur" class="inp" name="currency" value="IDR" maxlength="3" required />
				</div>
			</div>
			<div>
				<label class="lbl mb-1.5" for="b-ref">{t('fin.batchRef')}</label>
				<input id="b-ref" class="inp" name="batch_ref" maxlength="128" required />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label class="lbl mb-1.5" for="b-date">{t('fin.date')}</label>
					<input id="b-date" class="inp" type="date" name="settlement_date" value={today} required />
				</div>
				<div>
					<label class="lbl mb-1.5" for="b-net">{t('fin.reportedNet')}</label>
					<input id="b-net" class="inp" name="reported_net" inputmode="numeric" pattern="[0-9.]*" required />
				</div>
			</div>
			<div>
				<label class="lbl mb-1.5" for="b-note">{t('c.note')}</label>
				<input id="b-note" class="inp" name="note" maxlength="500" />
			</div>
			<button type="submit" class="btn btn-red" disabled={selected.length === 0}>{t('fin.recordBtn')}</button>
		</form>
	{/if}
</div>

<section class="card mb-6 overflow-hidden">
	<div class="p-5"><h2 class="text-sm font-bold">{t('fin.exceptions')}</h2></div>
	{#if data.exceptions === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.exceptions.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('fin.exNone')}</p>
	{:else}
		<ul class="divide-y divide-ink/10 text-xs">
			{#each data.exceptions as e (e.id)}
				<li class="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
					<div>
						<b>{e.gateway} · {e.reference}</b>
						<div class="muted">{t('fin.exDetail', { reported: formatIDR(e.details.reported_net), expected: formatIDR(e.details.expected_net), diff: formatIDR(e.details.difference) })}</div>
					</div>
					{#if data.canManage}
						<form method="POST" action="?/resolve" class="flex items-center gap-2" use:enhance>
							<input type="hidden" name="id" value={e.id} />
							<input class="inp w-64 py-2" name="note" placeholder={t('fin.resolveNote')} minlength="3" maxlength="1000" required aria-label={t('fin.resolveNote')} />
							<button class="btn btn-light" type="submit">{t('fin.resolve')}</button>
						</form>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<section class="card overflow-hidden">
	<div class="p-5"><h2 class="text-sm font-bold">{t('fin.batches')}</h2></div>
	{#if data.batches === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.batches.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('fin.batchesNone')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('fin.col.ref')}</th><th>{t('fin.gateway')}</th><th>{t('fin.col.date')}</th><th>{t('fin.col.count')}</th><th>{t('fin.col.net')}</th></tr></thead>
				<tbody>
					{#each data.batches as b (b.id)}
						<tr>
							<td class="font-bold">{b.batch_ref}</td>
							<td>{b.gateway}</td>
							<td class="muted">{b.settlement_date}</td>
							<td>{b.payment_count}</td>
							<td class="font-bold">{formatIDR(b.reported_net)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>
