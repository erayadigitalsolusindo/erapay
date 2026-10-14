<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../../lib/components/StateBadge.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
	const filters = ['submitted', 'under_review', 'action_required', 'approved', 'rejected', 'suspended'] as const;
	const pct = (bps: FormDataEntryValue | null) => (Number(bps) / 100).toString();
	const reviewable = ['submitted', 'under_review', 'action_required'];
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<section class="card overflow-hidden">
	<div class="flex flex-wrap items-center justify-between gap-3 p-5">
		<h2 class="text-sm font-bold">{t('onb.queue')}</h2>
		<nav class="flex flex-wrap gap-1.5" aria-label={t('fin.filter')}>
			{#each filters as s (s)}
				<a
					href={`?status=${s}`}
					aria-current={data.status === s ? 'true' : undefined}
					class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white"
					>{t(`onb.st.${s}` as const)}</a
				>
			{/each}
		</nav>
	</div>
	<p class="muted px-5 pb-4 text-[11px]">{t('onb.gatewayNote')}</p>
	{#if data.merchants === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.merchants.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('onb.none')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('c.created')}</th><th>{t('c.merchant')}</th><th>{t('onb.owner')}</th><th>{t('c.status')}</th><th>{t('c.actions')}</th></tr></thead>
				<tbody>
					{#each data.merchants as m (m.id)}
						<tr>
							<td class="muted">{fmt(m.created_at)}</td>
							<td>
								<div class="font-bold">{m.display_name}</div>
								<div class="muted text-[10px]">{m.account_type ? t(`onb.type.${m.account_type}` as const) : ''} · <code>{m.id.slice(0, 8)}</code></div>
							</td>
							<td>
								<div>{m.owner_name ?? '—'}</div>
								<div class="muted text-[10px]">{m.owner_email ?? ''}{m.contact_phone ? ` · ${m.contact_phone}` : ''}</div>
								<div class="mt-1 text-[10px] font-semibold {m.owner_email_verified ? 'text-emerald-700' : 'text-amber-700'}">{m.owner_email_verified ? t('onb.emailOk') : t('onb.emailNo')}</div>
								<div class="mt-0.5 text-[10px] font-semibold {m.identity_complete ? 'text-emerald-700' : 'text-amber-700'}">{m.identity_complete ? t('onb.identityOk') : t('onb.identityNo')}</div>
							</td>
							<td>
								<StateBadge kind="onb" value={m.status} />
								{#if m.review_note}<div class="muted mt-1 text-[10px]">{m.review_note}</div>{/if}
								{#if m.reviewed_at}<div class="muted mt-1 text-[10px]">{fmt(m.reviewed_at)}</div>{/if}
							</td>
							<td>
								<div class="mb-2"><a class="text-[11px] font-bold text-merah hover:underline" href={`/dashboard/finance/merchants/${encodeURIComponent(m.id)}`}>{t('onb.detail')}</a></div>
								{#if data.canReview && ['submitted', 'under_review'].includes(m.status)}
									<div class="mb-3 flex flex-wrap items-center gap-2">
										{#if m.status === 'submitted'}
											<form method="POST" action="?/startReview" use:enhance>
												<input type="hidden" name="id" value={m.id} />
												<button class="btn" type="submit">{t('onb.startReview')}</button>
											</form>
										{/if}
										<form method="POST" action="?/requestChanges" class="flex items-center gap-2" use:enhance>
											<input type="hidden" name="id" value={m.id} />
											<input class="inp w-44 py-2" name="note" placeholder={t('onb.changesNote')} minlength="3" maxlength="500" required aria-label={t('onb.changesNote')} />
											<button class="btn" type="submit">{t('onb.requestChanges')}</button>
										</form>
									</div>
								{/if}
								{#if reviewable.includes(m.status) && data.canApprove}
									{#if !m.owner_email_verified}<p class="mb-1 text-[10px] font-semibold text-amber-700">{t('onb.needVerify')}</p>{/if}
									<div class="flex flex-wrap items-start gap-3">
										<form
											method="POST"
											action="?/approve"
											class="flex flex-wrap items-center gap-2"
											use:enhance={({ formData, cancel }) => {
												const msg = t('onb.approveConfirm', { name: m.display_name, mdr: pct(formData.get('mdr_bps')), platform: pct(formData.get('platform_bps')) });
												if (!confirm(msg)) cancel();
											}}
										>
											<input type="hidden" name="id" value={m.id} />
											<input class="inp w-32 py-2" name="mdr_bps" type="number" min="0" max="10000" step="1" inputmode="numeric" required placeholder={t('onb.mdr')} aria-label={t('onb.mdr')} />
											<input class="inp w-44 py-2" name="platform_bps" type="number" min="0" max="10000" step="1" inputmode="numeric" required placeholder={t('onb.platform')} aria-label={t('onb.platform')} />
											<button class="btn btn-red" type="submit">{t('onb.approveBtn')}</button>
										</form>
										<form method="POST" action="?/reject" class="flex items-center gap-2" use:enhance>
											<input type="hidden" name="id" value={m.id} />
											<input class="inp w-44 py-2" name="note" placeholder={t('onb.noteRequired')} minlength="3" maxlength="500" required aria-label={t('onb.noteRequired')} />
											<button class="btn btn-danger" type="submit">{t('fin.reject')}</button>
										</form>
									</div>
									<p class="muted mt-1 max-w-sm text-[10px] whitespace-normal">{t('onb.bpsHint')}</p>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>
