<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import type { Key } from '../../../../lib/i18n/messages';
	import { formatIDR } from '../../../../lib/utils/money';

	let { data, form } = $props();
	const { t } = getI18n();
	const when = (iso: string) => new Date(iso).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
</script>

<h1 class="text-xl font-extrabold">{t('nav.fQrisgate')}</h1>
<p class="muted mt-1 mb-4 max-w-2xl text-xs">{t('nav.fQrisgate.desc')}</p>
{#if form?.message}<p class="mb-3 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

{#if data.topups === null}
	<p class="card p-5 text-xs font-semibold text-red-700" role="alert">{t('qg.err')}</p>
{:else if data.topups.length === 0}
	<p class="card p-5 text-xs">{t('qg.empty')}</p>
{:else}
	<div class="card overflow-x-auto p-5">
		<table class="w-full text-left text-xs">
			<thead><tr><th class="py-1">{t('qg.col.date')}</th><th>{t('qg.f.merchant')}</th><th>{t('qg.col.amount')}</th><th>{t('qg.col.status')}</th><th></th></tr></thead>
			<tbody>
				{#each data.topups as tp (tp.id)}
					<tr class="border-t border-ink/10 align-top">
						<td class="py-1.5">{when(tp.created_at)}</td>
						<td class="font-mono">{tp.merchant_id.slice(0, 8)}</td>
						<td>{formatIDR(tp.amount)}</td>
						<td>{t(`qg.st.${tp.status}` as Key)}{#if tp.bank_reference} · {tp.bank_reference}{/if}</td>
						<td>
							{#if tp.status === 'requested' && data.canManage}
								<form method="POST" action="?/confirm" use:enhance class="mb-1 flex gap-1">
									<input type="hidden" name="id" value={tp.id} />
									<input name="ref" required minlength="3" maxlength="128" placeholder={t('qg.f.ref')} class="inp" />
									<button class="font-bold text-emerald-700">{t('qg.f.confirm')}</button>
								</form>
								<form method="POST" action="?/reject" use:enhance class="flex gap-1">
									<input type="hidden" name="id" value={tp.id} />
									<input name="note" required minlength="3" maxlength="500" placeholder={t('qg.f.note')} class="inp" />
									<button class="font-bold text-merah">{t('qg.f.reject')}</button>
								</form>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
