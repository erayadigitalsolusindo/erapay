<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;

	const editable = () =>
		['submitted', 'under_review', 'action_required'].includes(data.profile?.onboarding_status ?? '');
	const fmtDob = (iso?: string) =>
		iso ? new Date(iso + 'T00:00:00').toLocaleDateString(i18n.locale, { dateStyle: 'medium' }) : '';
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
{#if form?.submitted}<p class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-xs font-semibold text-emerald-800" role="status">{t('onb.acct.submitted')}</p>{/if}
{#if form?.idSaved}<p class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-xs font-semibold text-emerald-800" role="status">{t('onb.acct.idSaved')}</p>{/if}

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

	<section class="card p-5 lg:col-span-2">
		<h2 class="text-sm font-bold">{t('onb.acct.identity')}</h2>
		{#if data.identity}
			<div class="mt-3 flex flex-wrap items-center gap-2 text-xs">
				<span class="font-bold">{data.identity.id_masked ? data.identity.id_masked : 'NIK belum diisi'}</span>
				<span class={`rounded-full px-2 py-0.5 text-[10px] font-bold ${data.identity.complete ? 'bg-emerald-100 text-emerald-800' : 'bg-amber-100 text-amber-800'}`}>
					{data.identity.complete ? t('onb.identityOk') : t('onb.identityNo')}
				</span>
			</div>
			{#if data.identity.place_of_birth}
				<p class="muted mt-2 text-xs">
					{data.identity.place_of_birth} · {fmtDob(data.identity.date_of_birth)} ·
					{data.identity.gender === 'male' ? t('reg.male') : data.identity.gender === 'female' ? t('reg.female') : ''}
				</p>
				<p class="muted text-xs">{data.identity.address}</p>
				{#if data.identity.business_nib}<p class="muted text-xs">NIB: {data.identity.business_nib}</p>{/if}
			{/if}
			<p class="muted mt-2 text-[11px]">{t('onb.acct.idMasked')}</p>
			{#if !data.identity.complete}
				<p class="mt-2 text-xs font-semibold text-amber-700" role="alert">{t('onb.acct.idIncomplete')}</p>
			{/if}
		{:else}
			<p class="muted mt-3 text-xs">{t('onb.acct.idIncomplete')}</p>
		{/if}

		{#if data.profile && editable()}
			<form method="POST" action="?/identity" class="mt-4 space-y-3" use:enhance>
				{#if data.profile?.account_type === 'business'}
					<div class="grid gap-3 sm:grid-cols-2">
						<label class="text-xs font-semibold">{t('reg.bizNpwp')}
							<input class="inp mt-1 block w-full py-2" name="biz_npwp" inputmode="numeric" placeholder={t('reg.npwpPh')} />
						</label>
						<label class="text-xs font-semibold">{t('reg.nib')}
							<input class="inp mt-1 block w-full py-2" name="nib" inputmode="numeric" placeholder={t('reg.nibPh')} />
						</label>
					</div>
				{/if}
				<div class="grid gap-3 sm:grid-cols-2">
					<label class="text-xs font-semibold">{t('reg.nik')}
						<input class="inp mt-1 block w-full py-2" name="nik" inputmode="numeric" placeholder={t('reg.nikPh')} required />
					</label>
					<label class="text-xs font-semibold">{t('reg.pob')}
						<input class="inp mt-1 block w-full py-2" name="pob" value={data.identity?.place_of_birth ?? ''} placeholder={t('reg.pobPh')} required />
					</label>
					<label class="text-xs font-semibold">{t('reg.dob')}
						<input class="inp mt-1 block w-full py-2" name="dob" type="date" value={data.identity?.date_of_birth ?? ''} required />
					</label>
					<label class="text-xs font-semibold">{t('reg.gender')}
						<select class="inp mt-1 block w-full py-2" name="gender" required>
							<option value="">{''}</option>
							<option value="male" selected={data.identity?.gender === 'male'}>{t('reg.male')}</option>
							<option value="female" selected={data.identity?.gender === 'female'}>{t('reg.female')}</option>
						</select>
					</label>
				</div>
				<label class="block text-xs font-semibold">{t('reg.address')}
					<textarea class="inp mt-1 block w-full py-2" name="address" rows="2" required>{data.identity?.address ?? ''}</textarea>
				</label>
				<label class="block text-xs font-semibold">{t('reg.npwp')}
					<input class="inp mt-1 block w-full py-2" name="npwp" inputmode="numeric" placeholder={t('reg.npwpPh')} />
				</label>
				<button class="btn btn-red" type="submit">{t('onb.acct.saveIdentity')}</button>
			</form>
		{:else if data.profile?.onboarding_status === 'approved' || data.profile?.onboarding_status === 'rejected'}
			<p class="muted mt-3 text-[11px]">{t('onb.acct.idLocked')}</p>
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
