<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import { formatIDR } from '../../../../lib/utils/money';
	let { data, form } = $props();
	const { t } = getI18n();
	const STATUSES = ['created', 'pending', 'succeeded', 'failed', 'expired', 'cancelled', 'unknown'];
</script>

<svelte:head><title>{t('pf.trx.title')}</title></svelte:head>

<div class="space-y-4">
	<div>
		<h1 class="text-lg font-extrabold">{t('pf.trx.title')}</h1>
		<p class="mt-1 text-xs">{t('pf.trx.hint')}</p>
	</div>

	<form method="GET" class="flex flex-wrap items-end gap-2">
		<input name="q" value={data.q} placeholder={t('pf.trx.search')} class="inp w-72 max-w-full" />
		<select name="status" class="inp">
			<option value="">{t('pf.trx.allStatus')}</option>
			{#each STATUSES as s (s)}<option value={s} selected={data.status === s}>{s}</option>{/each}
		</select>
		<button class="btn btn-light">{t('pf.trx.filter')}</button>
	</form>

	{#if form?.deleted}<p class="text-xs font-semibold text-emerald-700" role="status">{t('pf.trx.deleted')}</p>{/if}
	{#if form?.message}<p class="text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
	{#if data.error}<p class="text-xs font-semibold text-red-700" role="alert">{data.error}</p>{/if}

	<div class="card overflow-x-auto p-4">
		<table class="w-full text-left text-xs">
			<thead>
				<tr class="muted">
					<th class="py-1 pr-3">{t('pf.trx.created')}</th><th class="pr-3">{t('pf.trx.merchant')}</th><th class="pr-3">{t('pf.trx.ref')}</th>
					<th class="pr-3">{t('pf.trx.amount')}</th><th class="pr-3">{t('pf.trx.status')}</th><th class="pr-3">Gateway</th><th>{t('pf.trx.action')}</th>
				</tr>
			</thead>
			<tbody>
				{#each data.rows as r (r.id)}
					<tr class="border-t border-dashed align-top">
						<td class="py-2 pr-3 whitespace-nowrap">{new Date(r.created_at).toLocaleString()}</td>
						<td class="pr-3">{r.merchant_name}</td>
						<td class="pr-3 break-all">{r.merchant_reference}<div class="muted text-[10px]">{r.id}</div></td>
						<td class="pr-3 whitespace-nowrap">{formatIDR(r.amount)}</td>
						<td class="pr-3">{r.status}</td>
						<td class="pr-3">{r.gateway}</td>
						<td>
							{#if r.deletable}
								<form method="POST" action="?/delete" use:enhance class="flex gap-1">
									<input type="hidden" name="id" value={r.id} />
									<input name="reason" required minlength="3" maxlength="500" placeholder={t('pf.trx.reason')} class="inp w-40" />
									<button class="btn btn-light text-red-700" onclick={(e) => { if (!confirm(t('pf.trx.confirm'))) e.preventDefault(); }}>{t('pf.trx.delete')}</button>
								</form>
							{:else}
								<span class="muted">{t('pf.trx.keep')}</span>
							{/if}
						</td>
					</tr>
				{:else}
					<tr><td colspan="7" class="py-4 text-center muted">{t('pf.trx.empty')}</td></tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>
