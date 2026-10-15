<script lang="ts">
	import Combobox from '../../../lib/components/Combobox.svelte';
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';
	import { formatIDR } from '../../../lib/utils/money';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
	const verified = $derived((data.accounts ?? []).filter((a) => a.status === 'verified'));
	const bankName = $derived(new Map(data.banks.map((b) => [b.code, b.name])));
	const bankLabel = (code: string) => bankName.get(code) ?? code;
	const cards = $derived(
		data.balance
			? ([
					['bal.available', 'bal.available.hint', data.balance.available, true],
					['bal.pending', 'bal.pending.hint', data.balance.pending, false],
					['bal.inPayout', 'bal.inPayout.hint', data.balance.in_payout, false],
					['bal.onHold', 'bal.onHold.hint', data.balance.on_hold, false],
					['bal.payable', 'bal.payable.hint', data.balance.payable, false]
				] as const)
			: []
	);
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

{#if data.balance === null}
	<p class="mb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
{:else}
	<section class="mb-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-5" aria-label="Saldo">
		{#each cards as [label, hint, value, main] (label)}
			<div class="card p-4 {main ? 'border-emas' : ''}">
				<div class="muted text-[11px] font-bold uppercase">{t(label)}</div>
				<div class="mt-1 font-display text-xl font-extrabold">{formatIDR(value)}</div>
				<div class="muted mt-1 text-[11px]">{t(hint)}</div>
			</div>
		{/each}
	</section>
{/if}

<div class="mb-6 grid gap-5 lg:grid-cols-2">
	{#if data.canRequest}
		<form method="POST" action="?/request" class="card space-y-4 p-6" use:enhance>
			<h2 class="text-sm font-bold">{t('po.request')}</h2>
			<p class="muted text-xs">{t('po.note')}</p>
			<input type="hidden" name="idem" value={data.idem} />
			{#if verified.length === 0}
				<p class="text-xs font-semibold text-amber-700">{t('po.noAccounts')}</p>
			{:else}
				<div>
					<label class="lbl mb-1.5" for="acct">{t('po.account')}</label>
					<Combobox id="acct" name="bank_account_id" required value={verified[0]?.id ?? ''}
						options={verified.map((a) => ({ value: a.id, label: `${bankLabel(a.bank_code)} ${a.account_masked} · ${a.account_name}` }))} />
				</div>
				<div>
					<label class="lbl mb-1.5" for="amount">{t('po.amount')}</label>
					<input id="amount" class="inp" name="amount" inputmode="numeric" pattern="[0-9.]*" required />
				</div>
				<button type="submit" class="btn btn-red">{t('po.requestBtn')}</button>
			{/if}
		</form>
	{/if}

	{#if data.canManage}
		<section class="card space-y-4 p-6">
			<h2 class="text-sm font-bold">{t('bank.title')}</h2>
			<p class="muted text-xs">{t('bank.note')}</p>
			{#if data.accounts === null}
				<p class="text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
			{:else if data.accounts.length === 0}
				<p class="muted text-xs">{t('bank.none')}</p>
			{:else}
				<ul class="divide-y divide-ink/10 text-xs">
					{#each data.accounts.filter((a) => a.status !== 'disabled') as a (a.id)}
						<li class="flex flex-wrap items-center justify-between gap-2 py-2.5">
							<span><b>{bankLabel(a.bank_code)}</b> {a.account_masked} · {a.account_name}</span>
							<span class="flex items-center gap-2">
								<StateBadge kind="bank" value={a.status} />
								<form
									method="POST"
									action="?/disableAccount"
									use:enhance={({ cancel }) => {
										if (!confirm(t('bank.disableConfirm'))) cancel();
									}}
								>
									<input type="hidden" name="id" value={a.id} />
									<button type="submit" class="btn btn-danger">{t('bank.disable')}</button>
								</form>
							</span>
						</li>
					{/each}
				</ul>
			{/if}
			<form method="POST" action="?/addAccount" class="grid gap-3 border-t border-ink/10 pt-4 sm:grid-cols-2" use:enhance>
				<div>
					<label class="lbl mb-1.5" for="bank_code">{t('bank.code')}</label>
					<Combobox id="bank_code" name="bank_code" required placeholder={t('bank.pick')}
						options={data.banks.map((b) => ({ value: b.code, label: `${b.name} (${b.code})` }))} />
				</div>
				<div>
					<label class="lbl mb-1.5" for="account_number">{t('bank.number')}</label>
					<input id="account_number" class="inp" name="account_number" inputmode="numeric" pattern="[0-9]{'{5,32}'}" autocomplete="off" required />
				</div>
				<div class="sm:col-span-2">
					<label class="lbl mb-1.5" for="account_name">{t('bank.name')}</label>
					<input id="account_name" class="inp" name="account_name" maxlength="100" required />
				</div>
				<div class="sm:col-span-2"><button type="submit" class="btn btn-light">{t('bank.add')}</button></div>
			</form>
		</section>
	{/if}
</div>

<div class="card overflow-hidden">
	<div class="p-5"><h2 class="text-sm font-bold">{t('po.list')}</h2></div>
	{#if data.payouts === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.payouts.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('po.none')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('c.created')}</th><th>{t('c.amount')}</th><th>{t('c.status')}</th><th>{t('po.ref')}</th><th></th></tr></thead>
				<tbody>
					{#each data.payouts as p (p.id)}
						<tr>
							<td class="muted">{fmt(p.created_at)}</td>
							<td class="font-bold">{formatIDR(p.amount)}{#if p.dry_run}<span class="muted ml-2 text-[10px] font-normal">{t('c.dryRun')}</span>{/if}</td>
							<td><StateBadge kind="po" value={p.status} /></td>
							<td class="muted">{p.bank_reference ?? p.failure_reason ?? p.decision_note ?? '—'}</td>
							<td>
								{#if p.status === 'pending_approval' && data.canRequest}
									<form
										method="POST"
										action="?/cancel"
										use:enhance={({ cancel }) => {
											if (!confirm(t('po.cancelConfirm'))) cancel();
										}}
									>
										<input type="hidden" name="id" value={p.id} />
										<button type="submit" class="btn btn-danger">{t('po.cancel')}</button>
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
