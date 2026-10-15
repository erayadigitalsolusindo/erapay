<script lang="ts">
	import { withConfirm } from '../../../lib/ui/confirm';
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';
	import { formatIDR } from '../../../lib/utils/money';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
</script>

<p class="muted mb-5 max-w-2xl text-xs">{t('rf.note')}</p>
{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<div class="card overflow-hidden">
	<div class="p-5"><h2 class="text-sm font-bold">{t('rf.title')}</h2></div>
	{#if data.refunds === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.refunds.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('rf.none')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('c.created')}</th><th>{t('rf.payment')}</th><th>{t('c.amount')}</th><th>{t('c.reason')}</th><th>{t('c.status')}</th><th></th></tr></thead>
				<tbody>
					{#each data.refunds as r (r.id)}
						<tr>
							<td class="muted">{fmt(r.created_at)}</td>
							<td><a class="font-bold text-merah hover:underline" href="/dashboard/transactions/{r.payment_id}">{r.payment_id.slice(0, 8)}…</a></td>
							<td class="font-bold">{formatIDR(r.amount)}</td>
							<td class="max-w-64 truncate" title={r.reason}>{r.reason}</td>
							<td><StateBadge kind="rf" value={r.status} /></td>
							<td>
								{#if r.status === 'pending_approval' && data.canRequest}
									<form
										method="POST"
										action="?/cancel"
										use:enhance={withConfirm({ variant: 'danger', title: 'Batalkan refund', successMessage: 'Refund dibatalkan.', confirmText: 'Ya, batalkan', message: t('rf.cancelConfirm') })}
									>
										<input type="hidden" name="id" value={r.id} />
										<button type="submit" class="btn btn-danger">{t('rf.cancel')}</button>
									</form>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
