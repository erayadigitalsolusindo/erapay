<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../../lib/components/StateBadge.svelte';
	import { formatIDR } from '../../../../lib/utils/money';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
	const filters = ['', 'pending_approval', 'approved', 'refunded', 'failed', 'cancelled'] as const;
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<section class="card overflow-hidden">
	<div class="flex flex-wrap items-center justify-between gap-3 p-5">
		<h2 class="text-sm font-bold">{t('fin.refundQueue')}</h2>
		<nav class="flex flex-wrap gap-1.5" aria-label={t('fin.filter')}>
			{#each filters as s (s)}
				<a
					href={s ? `?status=${s}` : '?'}
					aria-current={data.status === s ? 'true' : undefined}
					class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white"
					>{s ? t(`rf.st.${s}` as const) : t('fin.allStatuses')}</a
				>
			{/each}
		</nav>
	</div>
	{#if data.refunds === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.refunds.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('rf.none')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('c.created')}</th><th>{t('c.merchant')}</th><th>{t('c.amount')}</th><th>{t('c.reason')}</th><th>{t('c.status')}</th><th>{t('c.actions')}</th></tr></thead>
				<tbody>
					{#each data.refunds as r (r.id)}
						<tr>
							<td class="muted">{fmt(r.created_at)}</td>
							<td><code>{r.merchant_id.slice(0, 8)}</code><div class="muted text-[10px]">{t('rf.payment')} {r.payment_id.slice(0, 8)}</div></td>
							<td class="font-bold">{formatIDR(r.amount)}</td>
							<td class="max-w-56 truncate" title={r.reason}>{r.reason}</td>
							<td>
								<StateBadge kind="rf" value={r.status} />
								{#if r.bank_reference}<div class="muted mt-1 text-[10px]">{t('po.ref')}: {r.bank_reference}</div>{/if}
								{#if r.failure_reason ?? r.decision_note}<div class="muted mt-1 text-[10px]">{r.failure_reason ?? r.decision_note}</div>{/if}
							</td>
							<td>
								{#if r.status === 'pending_approval' && data.canApprove}
									{#if r.requested_by === data.me}
										<span class="muted text-[11px]">{t('fin.sod')}</span>
									{:else}
										<div class="flex flex-wrap items-center gap-2">
											<form method="POST" action="?/approve" use:enhance>
												<input type="hidden" name="id" value={r.id} />
												<button class="btn btn-red" type="submit">{t('fin.approve')}</button>
											</form>
											<form method="POST" action="?/reject" class="flex items-center gap-2" use:enhance>
												<input type="hidden" name="id" value={r.id} />
												<input class="inp w-44 py-2" name="note" placeholder={t('fin.rejectNote')} minlength="3" maxlength="500" required aria-label={t('fin.rejectNote')} />
												<button class="btn btn-danger" type="submit">{t('fin.reject')}</button>
											</form>
										</div>
									{/if}
								{:else if r.status === 'approved' && data.canExecute}
									<div class="flex flex-wrap items-center gap-2">
										<form method="POST" action="?/markRefunded" class="flex items-center gap-2" use:enhance>
											<input type="hidden" name="id" value={r.id} />
											<input class="inp w-40 py-2" name="bank_reference" placeholder={t('fin.bankRef')} minlength="3" maxlength="128" required aria-label={t('fin.bankRef')} />
											<button class="btn btn-red" type="submit">{t('fin.markRefunded')}</button>
										</form>
										<form method="POST" action="?/markFailed" class="flex items-center gap-2" use:enhance>
											<input type="hidden" name="id" value={r.id} />
											<input class="inp w-40 py-2" name="reason" placeholder={t('fin.failReason')} minlength="3" maxlength="500" required aria-label={t('fin.failReason')} />
											<button class="btn btn-danger" type="submit">{t('fin.markFailed')}</button>
										</form>
									</div>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>
