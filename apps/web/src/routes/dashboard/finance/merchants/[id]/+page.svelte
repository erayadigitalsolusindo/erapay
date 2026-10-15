<script lang="ts">
	import { getI18n } from '../../../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../../../lib/components/StateBadge.svelte';
	import MerchantActions from '../../../../../lib/components/MerchantActions.svelte';
	import ImageLightbox from '../../../../../lib/components/kyc/ImageLightbox.svelte';
	import type { KycFileRef } from './+page.server';

	let { data } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	const fmt = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
	const fmtDob = (iso?: string) =>
		iso ? new Date(iso + 'T00:00:00').toLocaleDateString(i18n.locale, { dateStyle: 'medium' }) : '—';
	const gender = (g?: string) => (g === 'male' ? t('reg.male') : g === 'female' ? t('reg.female') : '—');
	const rupiah = (n: number) => new Intl.NumberFormat('id-ID').format(n);
	const m = $derived(data.merchant);
	const kyc = $derived(data.kyc);
	const isImg = (f: KycFileRef) => f.mime.startsWith('image/');
	let zoom = $state<KycFileRef | null>(null);
	const open = (f: KycFileRef) => (zoom = f);
</script>

{#snippet row(label: string, value: string | number | undefined | null, always = false)}
	{#if always || (value !== undefined && value !== null && value !== '')}
		{@const empty = value === undefined || value === null || value === ''}
		<div class="flex justify-between gap-4 py-1.5"><span class="muted">{label}</span><span class="text-right font-medium break-words {empty ? 'muted font-normal' : ''}">{empty ? '—' : value}</span></div>
	{/if}
{/snippet}

{#snippet file(label: string, f: KycFileRef | null | undefined, number?: string)}
	<div>
		<p class="muted mb-1 text-[11px] font-semibold">{label}</p>
		{#if f}
			{#if isImg(f)}
				<button type="button" class="block w-full cursor-zoom-in overflow-hidden rounded-lg border border-ink/15 bg-ink/5" aria-label={`Perbesar ${label}`} onclick={() => open(f)}>
					<img src={f.url} alt={label} class="h-36 w-full object-contain" loading="lazy" />
				</button>
			{:else}
				<a href={f.url} target="_blank" rel="noopener" class="flex items-center gap-2 rounded-lg border border-ink/15 px-3 py-3 text-xs font-bold text-merah hover:bg-ink/5">
					📄 {f.name || label} <span class="muted font-normal">· buka</span>
				</a>
			{/if}
			{#if number}<p class="mt-1 text-[11px] font-semibold">{number}</p>{/if}
		{:else}
			<p class="rounded-lg border border-dashed border-ink/20 px-3 py-3 text-[11px] text-amber-700">Belum diunggah</p>
		{/if}
	</div>
{/snippet}

<svelte:head><title>{t('onb.detailTitle')} · {m.display_name}</title></svelte:head>

<div class="mb-4 flex flex-wrap items-center gap-3">
	<a href="/dashboard/finance/merchants" class="text-[11px] font-bold text-merah hover:underline">← {t('onb.queue')}</a>
	<span class="font-bold">{t('onb.detailTitle')}</span>
	<span class="ml-auto"><MerchantActions m={{ id: m.id, display_name: m.display_name, status: m.status, owner_email_verified: m.owner_email_verified }} canReview={data.canReview} canApprove={data.canApprove} /></span>
</div>

<div class="grid gap-5 lg:grid-cols-2">
	<section class="card p-5">
		<h2 class="text-sm font-bold">{m.legal_name}</h2>
		<p class="mt-2 flex items-center gap-2 text-xs"><StateBadge kind="onb" value={m.status} /></p>
		{#if m.review_note}<p class="muted mt-2 text-xs">{m.review_note}</p>{/if}
		<div class="mt-3 text-xs">
			{@render row(t('onb.owner'), m.owner_name)}
			{@render row('Email', m.owner_email)}
			{@render row(t('onb.acct.phone'), m.contact_phone)}
			{@render row(t('c.created'), fmt(m.created_at))}
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
				{@render row(t('onb.detailIdNumber'), m.identity.id_number ?? m.identity.id_masked, true)}
				{@render row(t('onb.detailPob'), m.identity.place_of_birth, true)}
				{@render row(t('onb.detailDob'), m.identity.date_of_birth ? fmtDob(m.identity.date_of_birth) : '', true)}
				{@render row(t('onb.detailGender'), m.identity.gender ? gender(m.identity.gender) : '', true)}
				{@render row(t('onb.detailAddress'), m.identity.address, true)}
				{@render row(t('onb.detailNpwp'), m.identity.npwp ?? m.identity.npwp_masked, true)}
				{@render row(t('onb.detailBizNpwp'), m.identity.business_npwp ?? m.identity.business_npwp_masked, true)}
				{@render row(t('onb.detailNib'), m.identity.business_nib, true)}
			</div>
			<p class="muted mt-2 text-[11px]">{t('onb.detailRaw')}</p>
		{:else}
			<div class="mt-3 text-xs">
				{@render row(t('onb.detailIdNumber'), '', true)}
				{@render row(t('onb.detailPob'), '', true)}
				{@render row(t('onb.detailDob'), '', true)}
				{@render row(t('onb.detailGender'), '', true)}
				{@render row(t('onb.detailAddress'), '', true)}
				{@render row(t('onb.detailNpwp'), '', true)}
				{@render row(t('onb.detailBizNpwp'), '', true)}
				{@render row(t('onb.detailNib'), '', true)}
			</div>
			<p class="muted mt-2 text-[11px]">{t('onb.identityNo')}</p>
		{/if}
	</section>
</div>

{#if kyc}
	<div class="mt-5 grid gap-5 lg:grid-cols-2">
		<section class="card p-5">
			<h2 class="text-sm font-bold">1 · Perusahaan</h2>
			<div class="mt-3 text-xs">
				{@render row('Business ID', kyc.company.business_id as string)}
				{@render row('Jenis entitas', kyc.company.entity_type as string)}
				{@render row('Nama badan usaha', kyc.company.company_name as string)}
				{@render row('Telepon', kyc.company.phone as string)}
				{@render row('Email', kyc.company.email as string)}
				{@render row('Alamat', [kyc.company.address_detail, kyc.company.kelurahan, kyc.company.kecamatan, kyc.company.kota, kyc.company.provinsi, kyc.company.postal_code].filter(Boolean).join(', '))}
			</div>
			<!-- Ringkasan kelengkapan: mengisi ruang kartu dan memberi gambaran cepat sebelum memutuskan -->
			<div class="mt-4 rounded-xl border border-ink/10 bg-ink/[0.03] p-3">
				<p class="muted mb-2 text-[11px] font-bold uppercase tracking-wider">Kelengkapan data</p>
				<ul class="space-y-1.5 text-xs">
					{#each kyc.checklist ?? [] as c (c.section)}
						<li class="flex items-center gap-2">
							<span class="grid size-4 shrink-0 place-items-center rounded-full text-[9px] font-bold text-white {c.needs_revision ? 'bg-amber-500' : c.complete ? 'bg-emerald-600' : 'bg-ink/30'}">{c.needs_revision ? '!' : c.complete ? '✓' : '·'}</span>
							<span class="font-medium">{c.label}</span>
							<span class="muted ml-auto text-[11px]">{c.needs_revision ? 'perlu revisi' : c.complete ? 'lengkap' : 'belum lengkap'}</span>
						</li>
					{/each}
				</ul>
				{#if kyc.submitted_at || kyc.reviewed_at}
					<div class="mt-2 border-t border-ink/10 pt-2 text-[11px]">
						{#if kyc.submitted_at}<p class="muted">Dikirim: <span class="font-semibold text-ink">{fmt(kyc.submitted_at)}</span></p>{/if}
						{#if kyc.reviewed_at}<p class="muted">Ditinjau: <span class="font-semibold text-ink">{fmt(kyc.reviewed_at)}</span></p>{/if}
					</div>
				{/if}
			</div>
			{#if kyc.notes?.some((n) => !n.resolved)}
				<div class="mt-3 rounded-xl border border-amber-300 bg-amber-50 p-3 text-xs text-amber-900">
					<p class="mb-1 text-[11px] font-bold uppercase tracking-wider">Catatan revisi aktif</p>
					<ul class="list-disc space-y-0.5 pl-4">
						{#each kyc.notes.filter((n) => !n.resolved) as n (n.id)}<li>{n.note}</li>{/each}
					</ul>
				</div>
			{/if}
		</section>

		<section class="card p-5">
			<h2 class="text-sm font-bold">2 · Perwakilan bisnis</h2>
			<div class="mt-3 text-xs">
				{@render row('Nama', kyc.representative.name)}
				{@render row('Jabatan', kyc.representative.position)}
				{@render row('Telepon', kyc.representative.phone)}
				{@render row('Email', kyc.representative.email)}
				{@render row('Kewarganegaraan', kyc.representative.nationality)}
				{@render row('NIK (tersamar)', kyc.representative.id_masked)}
			</div>
			<div class="mt-3 grid grid-cols-2 gap-3">
				{@render file('KTP — depan', kyc.representative.ktp)}
				{@render file('KTP — belakang', kyc.representative.ktp_back)}
			</div>
		</section>

		<section class="card p-5">
			<h2 class="text-sm font-bold">3 · Merek</h2>
			<div class="mt-3 text-xs">
				{@render row('Brand ID', kyc.brand.brand_id)}
				{@render row('Nama merek', kyc.brand.name)}
				{@render row('Kategori', kyc.brand.category)}
				{@render row('Deskripsi', kyc.brand.description)}
				{@render row('Perkiraan nilai / bulan', kyc.brand.monthly_amount ? `Rp ${rupiah(kyc.brand.monthly_amount)}` : '')}
				{@render row('Perkiraan transaksi / bulan', kyc.brand.monthly_count ? rupiah(kyc.brand.monthly_count) : '')}
			</div>
			<div class="mt-3">{@render file('Logo merek', kyc.brand.logo)}</div>
			<p class="muted mb-1 mt-3 text-[11px] font-semibold">Bukti bisnis ({kyc.brand.evidences.length})</p>
			{#if kyc.brand.evidences.length}
				<div class="grid grid-cols-3 gap-2 sm:grid-cols-4">
					{#each kyc.brand.evidences as ev (ev.id)}
						<button type="button" class="aspect-square cursor-zoom-in overflow-hidden rounded-lg border border-ink/15" aria-label={`Perbesar ${ev.name}`} onclick={() => open(ev)}>
							<img src={ev.url} alt={ev.name} class="size-full object-cover" loading="lazy" />
						</button>
					{/each}
				</div>
			{:else}
				<p class="text-[11px] text-amber-700">Belum ada bukti.</p>
			{/if}
			{#if kyc.brand.links?.length}
				<p class="muted mb-1 mt-3 text-[11px] font-semibold">Tautan jualan</p>
				<ul class="space-y-1 text-xs">
					{#each kyc.brand.links as l (l.id)}<li><a href={l.url} target="_blank" rel="noopener noreferrer nofollow" class="break-all text-merah hover:underline">{l.url}</a></li>{/each}
				</ul>
			{/if}
		</section>

		<section class="card p-5">
			<h2 class="text-sm font-bold">4 · Dokumen legal</h2>
			{#if kyc.company.entity_type === 'Perorangan'}
				<div class="empty mt-4 grid place-items-center gap-3 rounded-xl border border-dashed border-ink/20 px-4 py-8 text-center">
					<span class="empty-ring relative grid size-16 place-items-center rounded-full bg-emerald-50 text-emerald-600">
						<svg viewBox="0 0 24 24" class="empty-float size-8" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 4 6v6c0 4.5 3.2 8 8 9 4.8-1 8-4.5 8-9V6l-8-3Z" /><path d="m9 12 2.2 2.2L15.5 10" /></svg>
					</span>
					<div>
						<p class="text-sm font-bold">Tidak ada dokumen badan usaha</p>
						<p class="muted mt-1 text-xs">Entitas <b>Perorangan</b> tidak diwajibkan mengunggah NPWP badan, akta, SK, NIB, maupun izin usaha. Cukup periksa KTP perwakilan.</p>
					</div>
				</div>
			{:else}
				<div class="mt-3 grid grid-cols-2 gap-3">
					{#each data.docs as d (d.key)}{@render file(d.label, d.file, d.number)}{/each}
				</div>
				{#if data.docs.length === 0}<p class="text-[11px] text-amber-700">Belum ada dokumen.</p>{/if}
			{/if}
		</section>
	</div>
{:else}
	<p class="muted mt-5 text-xs">Data KYC wizard belum tersedia untuk merchant ini.</p>
{/if}

{#if zoom}<ImageLightbox src={zoom.url} alt={zoom.name} onclose={() => (zoom = null)} />{/if}

<style>
	.empty-float {
		animation: empty-float 3s ease-in-out infinite;
	}
	.empty-ring::before,
	.empty-ring::after {
		content: '';
		position: absolute;
		inset: 0;
		border-radius: 9999px;
		border: 2px solid currentColor;
		opacity: 0;
		animation: empty-ping 2.8s ease-out infinite;
	}
	.empty-ring::after {
		animation-delay: 1.4s;
	}
	@keyframes empty-float {
		0%, 100% { transform: translateY(0); }
		50% { transform: translateY(-4px); }
	}
	@keyframes empty-ping {
		0% { transform: scale(1); opacity: 0.45; }
		100% { transform: scale(1.7); opacity: 0; }
	}
	@media (prefers-reduced-motion: reduce) {
		.empty-float, .empty-ring::before, .empty-ring::after { animation: none; }
	}
</style>
