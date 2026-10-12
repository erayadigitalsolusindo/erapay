<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../../lib/components/StateBadge.svelte';
	import { formatIDR } from '../../../../lib/utils/money';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
	const filters = ['', 'pending_approval', 'approved', 'paid', 'failed', 'cancelled'] as const;
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
{#if form?.dest}
	<section class="card mb-5 max-w-xl space-y-2 border-emas p-6" role="status">
		<h2 class="text-sm font-bold">{t('fin.dest')}</h2>
		<p class="muted text-xs">{t('fin.destNote')}</p>
		<dl class="grid grid-cols-[auto_1fr] gap-x-5 gap-y-1 text-xs">
			<dt class="muted">{t('bank.code')}</dt><dd class="font-bold">{form.dest.bank_code}</dd>
			<dt class="muted">{t('bank.number')}</dt><dd class="font-bold select-all"><code>{form.dest.account_number}</code></dd>
			<dt class="muted">{t('bank.name')}</dt><dd class="font-bold">{form.dest.account_name}</dd>
		</dl>
	</section>
{/if}

<section class="card mb-6 overflow-hidden">
	<div class="p-5">
		<h2 class="text-sm font-bold">{t('fin.bankPending')}</h2>
		<p class="muted mt-1 text-xs">{t('fin.verifyHint')}</p>
	</div>
	{#if data.accounts === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.accounts.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('fin.bankPendingNone')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('c.created')}</th><th>{t('c.merchant')}</th><th>{t('bank.code')}</th><th>{t('bank.number')}</th><th>{t('bank.name')}</th><th></th></tr></thead>
				<tbody>
					{#each data.accounts as a (a.id)}
						<tr>
							<td class="muted">{fmt(a.created_at)}</td>
							<td><code>{a.merchant_id.slice(0, 8)}</code></td>
							<td>{a.bank_code}</td>
							<td>{a.account_masked}</td>
							<td class="font-bold">{a.account_name}</td>
							<td>
								{#if data.canVerify}
									<form method="POST" action="?/verify" use:enhance>
										<input type="hidden" name="id" value={a.id} />
										<button class="btn btn-light" type="submit">{t('fin.verify')}</button>
									</form>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>

<section class="card overflow-hidden">
	<div class="flex flex-wrap items-center justify-between gap-3 p-5">
		<h2 class="text-sm font-bold">{t('fin.queue')}</h2>
		<nav class="flex flex-wrap gap-1.5" aria-label={t('fin.filter')}>
			{#each filters as s (s)}
				<a
					href={s ? `?status=${s}` : '?'}
					aria-current={data.status === s ? 'true' : undefined}
					class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white"
					>{s ? t(`po.st.${s}` as const) : t('fin.allStatuses')}</a
				>
			{/each}
		</nav>
	</div>
	{#if data.payouts === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.payouts.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('fin.queueNone')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('c.created')}</th><th>{t('c.merchant')}</th><th>{t('c.amount')}</th><th>{t('c.status')}</th><th>{t('c.actions')}</th></tr></thead>
				<tbody>
					{#each data.payouts as p (p.id)}
						<tr>
							<td class="muted">{fmt(p.created_at)}</td>
							<td><code>{p.merchant_id.slice(0, 8)}</code></td>
							<td class="font-bold">{formatIDR(p.amount)}{#if p.dry_run}<div class="muted text-[10px] font-normal">{t('c.dryRun')}</div>{/if}</td>
							<td>
								<StateBadge kind="po" value={p.status} />
								{#if p.bank_reference}<div class="muted mt-1 text-[10px]">{t('po.ref')}: {p.bank_reference}</div>{/if}
								{#if p.failure_reason ?? p.decision_note}<div class="muted mt-1 text-[10px]">{p.failure_reason ?? p.decision_note}</div>{/if}
							</td>
							<td>
								{#if p.status === 'pending_approval' && data.canApprove}
									{#if p.requested_by === data.me}
										<span class="muted text-[11px]">{t('fin.sod')}</span>
									{:else}
										<div class="flex flex-wrap items-center gap-2">
											<form method="POST" action="?/approve" use:enhance>
												<input type="hidden" name="id" value={p.id} />
												<button class="btn btn-red" type="submit">{t('fin.approve')}</button>
											</form>
											<form method="POST" action="?/reject" class="flex items-center gap-2" use:enhance>
												<input type="hidden" name="id" value={p.id} />
												<input class="inp w-44 py-2" name="note" placeholder={t('fin.rejectNote')} minlength="3" maxlength="500" required aria-label={t('fin.rejectNote')} />
												<button class="btn btn-danger" type="submit">{t('fin.reject')}</button>
											</form>
										</div>
									{/if}
								{:else if p.status === 'approved' && data.canExecute}
									<div class="flex flex-wrap items-center gap-2">
										<form method="POST" action="?/reveal" use:enhance>
											<input type="hidden" name="id" value={p.id} />
											<button class="btn btn-light" type="submit">{t('fin.reveal')}</button>
										</form>
										<form method="POST" action="?/markPaid" class="flex items-center gap-2" use:enhance>
											<input type="hidden" name="id" value={p.id} />
											<input class="inp w-40 py-2" name="bank_reference" placeholder={t('fin.bankRef')} minlength="3" maxlength="128" required aria-label={t('fin.bankRef')} />
											<button class="btn btn-red" type="submit">{t('fin.markPaid')}</button>
										</form>
										<form method="POST" action="?/markFailed" class="flex items-center gap-2" use:enhance>
											<input type="hidden" name="id" value={p.id} />
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
