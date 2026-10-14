<script lang="ts">
	import { getI18n } from '../../../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../../../lib/components/StateBadge.svelte';

	let { data } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
	const fmtDob = (iso?: string) =>
		iso ? new Date(iso + 'T00:00:00').toLocaleDateString(i18n.locale, { dateStyle: 'medium' }) : '—';
	const gender = (g?: string) => (g === 'male' ? t('reg.male') : g === 'female' ? t('reg.female') : '—');
	const row = (label: string, value: string | undefined) => (value ? `<div class="flex justify-between gap-4 py-1.5"><span class="muted">${label}</span><span class="text-right font-medium">${value}</span></div>` : '');
	const m = $derived(data.merchant);
</script>

<svelte:head><title>{t('onb.detailTitle')} · {m.display_name}</title></svelte:head>

<div class="mb-4 flex items-center gap-3">
	<a href="/dashboard/finance/merchants" class="text-[11px] font-bold text-merah hover:underline">← {t('onb.queue')}</a>
	<span class="font-bold">{t('onb.detailTitle')}</span>
</div>

<div class="grid gap-5 lg:grid-cols-2">
	<section class="card p-5">
		<h2 class="text-sm font-bold">{m.legal_name}</h2>
		<p class="mt-2 flex items-center gap-2 text-xs"><StateBadge kind="onb" value={m.status} /></p>
		{#if m.review_note}<p class="muted mt-2 text-xs">{m.review_note}</p>{/if}
		<div class="mt-3 space-y-0.5 text-xs">
			{@html row(t('onb.owner'), m.owner_name ?? '')}
			{@html row('Email', m.owner_email ?? '')}
			{@html row(t('onb.acct.phone'), m.contact_phone ?? '')}
			{@html row(t('c.created'), fmt(m.created_at))}
		</div>
		<div class="mt-3 flex flex-wrap gap-2 text-[10px] font-bold">
			<span class="rounded-full px-2 py-0.5 {m.owner_email_verified ? 'bg-emerald-100 text-emerald-800' : 'bg-amber-100 text-amber-800'}">{m.owner_email_verified ? t('onb.emailOk') : t('onb.emailNo')}</span>
			<span class="rounded-full px-2 py-0.5 {m.identity_complete ? 'bg-emerald-100 text-emerald-800' : 'bg-amber-100 text-amber-800'}">{m.identity_complete ? t('onb.identityOk') : t('onb.identityNo')}</span>
		</div>
	</section>

	<section class="card p-5">
		<h2 class="text-sm font-bold">{t('onb.detailIdentity')}</h2>
		{#if m.identity}
			<div class="mt-3 text-xs">
				{@html row(t('onb.detailIdNumber'), m.identity.id_number ?? m.identity.id_masked)}
				{@html row(t('onb.detailPob'), m.identity.place_of_birth)}
				{@html row(t('onb.detailDob'), fmtDob(m.identity.date_of_birth))}
				{@html row(t('onb.detailGender'), gender(m.identity.gender))}
				{@html row(t('onb.detailAddress'), m.identity.address)}
				{@html row(t('onb.detailNpwp'), m.identity.npwp ?? m.identity.npwp_masked)}
				{@html row(t('onb.detailBizNpwp'), m.identity.business_npwp ?? m.identity.business_npwp_masked)}
				{@html row(t('onb.detailNib'), m.identity.business_nib)}
			</div>
			<p class="muted mt-2 text-[11px]">{t('onb.detailRaw')}</p>
		{:else}
			<p class="muted mt-3 text-xs">{t('onb.identityNo')}</p>
		{/if}
	</section>
</div>
