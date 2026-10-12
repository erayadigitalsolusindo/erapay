<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';

	let { data, form } = $props();
	const { t } = getI18n();
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
{#if form?.submitted}<p class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-xs font-semibold text-emerald-800" role="status">{t('onb.acct.submitted')}</p>{/if}

<div class="grid gap-5 lg:grid-cols-2">
	<section class="card p-5">
		<h2 class="text-sm font-bold">{t('onb.acct.status')}</h2>
		{#if data.profile === null}
			<p class="mt-3 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
		{:else}
			<p class="mt-3 flex items-center gap-2"><span class="font-bold">{data.profile.display_name}</span> <StateBadge kind="onb" value={data.profile.onboarding_status} /></p>
			{#if data.profile.review_note}<p class="muted mt-2 text-xs">{data.profile.review_note}</p>{/if}
			{#if data.profile.onboarding_status === 'approved'}<p class="muted mt-3 text-[11px]">{t('onb.gatewayNote')}</p>{/if}
		{/if}
	</section>

	<section class="card p-5">
		<h2 class="text-sm font-bold">{t('onb.acct.email')}</h2>
		{#if data.emailVerified}
			<p class="mt-3 text-xs font-semibold text-emerald-700">{t('onb.acct.emailOk', { email: data.email })}</p>
		{:else}
			<p class="mt-3 text-xs">{t('onb.acct.emailPending', { email: data.email })}</p>
			{#if form?.resent}<p class="mt-2 text-xs font-semibold text-emerald-700" role="status">{t('onb.acct.resent')}</p>{/if}
			<form method="POST" action="?/resend" class="mt-3" use:enhance>
				<button class="btn btn-red" type="submit">{t('onb.acct.resend')}</button>
			</form>
		{/if}
	</section>

	{#if data.profile?.onboarding_status === 'action_required'}
		<section class="card p-5 lg:col-span-2">
			<h2 class="text-sm font-bold">{t('onb.acct.fix')}</h2>
			<p class="muted mt-1 text-xs">{t('onb.acct.fixNote')}</p>
			<form method="POST" action="?/resubmit" class="mt-4 flex flex-wrap items-end gap-3" use:enhance>
				<label class="text-xs font-semibold">{t('onb.acct.legal')}
					<input class="inp mt-1 block w-64 py-2" name="legal_name" maxlength="120" placeholder={data.profile.legal_name} />
				</label>
				<label class="text-xs font-semibold">{t('onb.acct.phone')}
					<input class="inp mt-1 block w-48 py-2" name="contact_phone" inputmode="tel" placeholder="0812 3456 7890" />
				</label>
				<button class="btn btn-red" type="submit">{t('onb.acct.submit')}</button>
			</form>
		</section>
	{/if}
</div>
