<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { enhance, type SubmitFunction } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import StatusBadge from '../../../lib/components/StatusBadge.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';
	import { formatIDR } from '../../../lib/utils/money';
	import type { Key } from '../../../lib/i18n/messages';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Jakarta' });
	const filtered = $derived(Object.values(data.filters).some(Boolean));

	// Id baris yang sedang di-cek ke gateway: tombol dinonaktifkan selama proses.
	let submitting = $state('');
	// Pakai enhance agar status di-refresh setelah inquiry; kunci submit ganda.
	const inquiryEnhance: SubmitFunction = (input) => {
		const id = String(input.formData.get('id') ?? '').trim();
		submitting = id;
		return async ({ update }) => {
			try {
				await update({ reset: false });
				await invalidateAll();
			} finally {
				submitting = '';
			}
		};
	};

	// Tautan "lebih lama" mempertahankan filter; kursor dari server bersifat opak.
	const nextHref = $derived.by(() => {
		const q = new URLSearchParams();
		if (data.filters.status) q.set('status', data.filters.status);
		if (data.filters.ref) q.set('ref', data.filters.ref);
		if (data.filters.from) q.set('from', data.filters.from);
		if (data.filters.to) q.set('to', data.filters.to);
		if (data.next) q.set('cursor', data.next);
		return `?${q}`;
	});
	const firstHref = $derived.by(() => {
		const q = new URLSearchParams();
		for (const [k, v] of Object.entries(data.filters)) if (v) q.set(k, v);
		return q.size ? `?${q}` : '?';
	});
</script>

<div class="mb-5 flex flex-wrap items-center justify-between gap-3">
	<h2 class="font-bold">{t('trx.list')}</h2>
	<a href="/dashboard/transactions/new" class="btn btn-red">＋ {t('trx.new')}</a>
</div>

<form method="GET" class="card mb-5 grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_1fr_1fr_auto]">
	<div>
		<label class="lbl mb-1.5" for="f-status">{t('trx.filter.status')}</label>
		<select id="f-status" name="status" class="inp">
			<option value="">{t('trx.filter.all')}</option>
			{#each data.statuses as s (s)}<option value={s} selected={data.filters.status === s}>{t(`status.${s}` as Key)}</option>{/each}
		</select>
	</div>
	<div>
		<label class="lbl mb-1.5" for="f-ref">{t('trx.filter.ref')}</label>
		<input id="f-ref" name="ref" class="inp" maxlength="64" value={data.filters.ref} />
	</div>
	<div>
		<label class="lbl mb-1.5" for="f-from">{t('trx.filter.from')}</label>
		<input id="f-from" name="from" type="date" class="inp" value={data.filters.from} />
	</div>
	<div>
		<label class="lbl mb-1.5" for="f-to">{t('trx.filter.to')}</label>
		<input id="f-to" name="to" type="date" class="inp" value={data.filters.to} />
	</div>
	<div class="flex items-end gap-2">
		<button type="submit" class="btn btn-red">{t('trx.filter.apply')}</button>
		{#if filtered}<a href="?" class="btn btn-light">{t('trx.filter.reset')}</a>{/if}
	</div>
</form>

<div class="card overflow-hidden">
	{#if form?.done}
		<p class="border-b border-ink/10 bg-emerald-50 px-5 py-3 text-xs font-semibold text-emerald-700" role="status">{t('trx.inquiryDone')}</p>
	{:else if form?.message}
		<p class="border-b border-ink/10 bg-rose-50 px-5 py-3 text-xs font-semibold text-red-700" role="alert">{form.message}</p>
	{/if}
	{#if data.payments === null}
		<p class="p-5 text-xs font-semibold text-red-700" role="alert">{t('trx.loadFail')}</p>
	{:else if data.payments.length === 0}
		<p class="muted p-5 text-xs">{filtered ? t('trx.noMatch') : t('trx.listEmpty')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead>
					<tr><th>{t('c.created')}</th><th>{t('trx.ref')}</th><th>{t('c.amount')}</th><th>{t('trx.f.method')}</th><th>{t('c.status')}</th><th>{t('trx.f.settlement')}</th><th class="text-right">{t('trx.f.actions')}</th></tr>
				</thead>
				<tbody>
					{#each data.payments as p (p.id)}
						<tr>
							<td class="muted">{fmt(p.created_at)}</td>
							<td><a class="font-bold text-merah hover:underline" href="/dashboard/transactions/{p.id}">{p.merchant_reference}</a></td>
							<td class="font-bold">
								{formatIDR(p.amount)}
								{#if p.refunded_amount}<div class="muted text-[10px] font-normal">{t('trx.f.refunded')}: {formatIDR(p.refunded_amount)}</div>{/if}
							</td>
							<td>{p.payment_method}</td>
							<td><StatusBadge status={p.status} /></td>
							<td>{#if p.settlement_status && p.settlement_status !== 'not_expected'}<StateBadge kind="settle" value={p.settlement_status} />{:else}<span class="muted">—</span>{/if}</td>
							<td class="text-right">
								{#if p.status === 'created' || p.status === 'pending' || p.status === 'unknown'}
									<form method="POST" action="?/inquiry" use:enhance={inquiryEnhance}>
										<input type="hidden" name="id" value={p.id} />
										<button type="submit" class="btn btn-light" disabled={submitting === p.id}>🔄 {t('trx.inquiryBtn')}</button>
									</form>
								{:else}
									<span class="muted text-[11px]">—</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<div class="flex items-center justify-between gap-3 border-t border-ink/10 px-5 py-3 text-xs font-bold">
			{#if data.paged}<a class="text-merah hover:underline" href={firstHref}>{t('trx.first')}</a>{:else}<span class="muted font-normal">{t('trx.dates')}</span>{/if}
			{#if data.next}<a class="text-merah hover:underline" href={nextHref}>{t('trx.next')}</a>{/if}
		</div>
	{/if}
</div>
