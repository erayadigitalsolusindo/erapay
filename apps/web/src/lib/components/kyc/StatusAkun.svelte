<script lang="ts">
	// Halaman Status Akun: badge status, checklist verifikasi, catatan reviewer, dan
	// seluruh bagian isian dalam MODE BACA — memakai komponen per-bagian yang SAMA
	// dengan wizard, cukup di-locked (read-only, tanpa api eksternal).
	import { getI18n } from '../../i18n/i18n.svelte';
	import KycStatusBadge from './KycStatusBadge.svelte';
	import CompanySection from './CompanySection.svelte';
	import RepresentativeSection from './RepresentativeSection.svelte';
	import BrandSection from './BrandSection.svelte';
	import DocumentsSection from './DocumentsSection.svelte';
	import type { Profile } from '../../server/kyc';
	import type { SectionApi } from './types';

	// Api stsb: StatusAkun hanya baca. Tidak ada kontak server dari komponen ini.
	const NOOP: SectionApi = {
		save: async () => ({ ok: true }),
		submit: async () => ({ ok: true }),
		uploadDoc: async () => ({ ok: true }),
		uploadLogo: async () => ({ ok: true }),
		addEvidence: async () => ({ ok: true }),
		deleteEvidence: async () => ({ ok: true }),
		regions: async () => []
	};

	const { t, tx } = getI18n();
	let { profile, wizardHref = '/dashboard/onboarding' }: { profile: Profile; wizardHref?: string } = $props();

	const check = $derived(profile.checklist ?? []);
	const doneN = $derived(check.filter((c) => c.complete).length);

	const ctaHref = $derived(
		profile.status === 'draft' || profile.status === 'action_required' || profile.status === 'rejected' ? wizardHref : ''
	);
	const ctaLabel = $derived(
		profile.status === 'draft'
			? t('ky.st.cta.draft')
			: profile.status === 'rejected'
				? t('ky.st.cta.rejected')
				: profile.status === 'action_required'
					? t('ky.st.cta.fix')
					: ''
	);
	const needsAction = $derived(
		profile.status === 'draft' || profile.status === 'action_required' || profile.status === 'rejected'
	);
	const approved = $derived(profile.status === 'approved' && !profile.recheck);
</script>

<div class="w-full">
	<!-- header -->
	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<div>
			<h1 class="text-xl font-bold text-[#1A2530]">{t('ky.st.title')}</h1>
			<p class="text-[13px] text-[#7A8894]">{t('ky.st.sub')}</p>
		</div>
		<KycStatusBadge status={profile.status} recheck={profile.recheck} />
	</div>

	<!-- banner status -->
	{#if profile.recheck}
		<div class="mb-4 rounded-md bg-[#FFF4E0] p-3 text-[13px] leading-relaxed text-[#8A5A00]">
			{t('ky.st.recheck')}
			{#if profile.recheck_policy === 'closed'}
				{t('ky.st.recheckClosed')}
			{:else}
				{t('ky.st.recheckOpen')}
			{/if}
		</div>
	{:else if approved}
		<div class="mb-4 rounded-md bg-[#E4F5EC] p-3 text-[13px] leading-relaxed text-[#1D7A46]">
			<b>{t('ky.st.okTitle')}</b> {t('ky.st.okBody')}
		</div>
	{:else if profile.status === 'rejected'}
		<div class="mb-4 rounded-md bg-[#FDF4F4] p-3 text-[13px] leading-relaxed text-[#6B3A3A]">
			<b>{t('ky.st.rejTitle')}</b> {t('ky.st.rejBody')}
			<a class="underline" href={wizardHref}>{t('ky.st.openForm')}</a> {t('ky.st.rejTail')}
		</div>
	{:else if profile.status === 'action_required'}
		<div class="mb-4 rounded-md bg-[#FDF4F4] p-3 text-[13px] leading-relaxed text-[#6B3A3A]">
			<b>{t('kw.fixTitle')}</b> {t('ky.st.fixBody')}
			<a class="underline" href={wizardHref}>{t('ky.st.openForm')}</a> {t('ky.st.fixTail')}
		</div>
	{:else if profile.locked}
		<div class="mb-4 rounded-md bg-[#E7F0FB] p-3 text-[13px] leading-relaxed text-[#1D4F91]">
			{t('ky.st.locked')}
		</div>
	{/if}
	{#if profile.review_note}
		<div class="mb-4 rounded-md border border-[#F3C1C1] bg-[#FDF4F4] p-3 text-[13px] text-[#6B3A3A]">
			<b>{t('ky.st.rejNote')}</b> {profile.review_note}
		</div>
	{/if}

	<!-- checklist -->
	<div class="mb-6 rounded-xl border border-[#E5E8ED] bg-white p-5 shadow-sm">
		<div class="mb-3 flex items-center justify-between">
			<h2 class="text-base font-bold text-[#1A2530]">{t('ky.st.checklist')}</h2>
			<span class="text-[12px] font-semibold text-[#7A8894]">{doneN}/{check.length}</span>
		</div>
		<div class="mb-4 h-1.5 w-full overflow-hidden rounded-full bg-[#E5E8ED]">
			<div class="h-full rounded-full bg-[#1D7A46] transition-all" style="width: {check.length ? (doneN / check.length) * 100 : 0}%"></div>
		</div>
		<ul class="grid grid-cols-1 gap-2 sm:grid-cols-2">
			{#each check as c (c.section)}
				<li
					class="flex items-center gap-2.5 rounded-md border px-3 py-2.5
					{c.complete ? 'border-[#D5EDE0] bg-[#F2FAF6]' : c.needs_revision ? 'border-[#F3C1C1] bg-[#FDF4F4]' : 'border-[#E5E8ED] bg-[#F7F9FB]'}"
				>
					<span
						class="grid size-6 shrink-0 place-items-center rounded-full
						{c.complete ? 'bg-[#1D7A46] text-white' : c.needs_revision ? 'bg-[#D32F2F] text-white' : 'border border-[#CBD2D9]'}"
					>
						{#if c.complete}
							<svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="3"><path d="m5 12 5 5L20 7" /></svg>
						{:else if c.needs_revision}
							!
						{/if}
					</span>
					<span class="text-[13px] font-semibold text-[#1A2530]">{tx(`kw.${c.section}`, c.label)}</span>
					<span class="ml-auto text-[10px] font-bold uppercase tracking-wide
						{c.complete ? 'text-[#1D7A46]' : c.needs_revision ? 'text-[#D32F2F]' : 'text-[#CBD2D9]'}">
						{c.complete ? t('ky.st.complete') : c.needs_revision ? t('ky.st.revision') : t('ky.st.todo')}
					</span>
				</li>
			{/each}
		</ul>
	</div>

	<!-- CTA -->
	{#if needsAction}
		<a
			href={ctaHref}
			class="mb-6 block cursor-pointer rounded-md bg-[#D32F2F] px-5 py-3 text-center text-[14px] font-bold tracking-wide text-white shadow-sm transition hover:bg-[#B02424]"
		>
			{ctaLabel}
		</a>
	{/if}

	<!-- isian (mode baca) -->
	<div class="mt-6 space-y-6">
		<section class="rounded-xl border border-[#E5E8ED] bg-white p-5 shadow-sm">
			<h2 class="mb-4 text-base font-bold text-[#1A2530]">{t('ky.st.s1')}</h2>
			<CompanySection data={profile.company} notes={profile.notes} locked api={NOOP} errors={{}} />
		</section>
		<section class="rounded-xl border border-[#E5E8ED] bg-white p-5 shadow-sm">
			<h2 class="mb-4 text-base font-bold text-[#1A2530]">{t('ky.st.s2')}</h2>
			<RepresentativeSection data={profile.representative} notes={profile.notes} locked ktp={profile.representative.ktp} ktpBack={profile.representative.ktp_back} api={NOOP} errors={{}} />
		</section>
		<section class="rounded-xl border border-[#E5E8ED] bg-white p-5 shadow-sm">
			<h2 class="mb-4 text-base font-bold text-[#1A2530]">{t('ky.st.s3')}</h2>
			<BrandSection data={profile.brand} notes={profile.notes} locked api={NOOP} errors={{}} />
		</section>
		<section class="rounded-xl border border-[#E5E8ED] bg-white p-5 shadow-sm">
			<h2 class="mb-4 text-base font-bold text-[#1A2530]">{t('ky.st.s4')}</h2>
			<DocumentsSection documents={profile.documents} notes={profile.notes} locked api={NOOP} errors={{}} approved={profile.status === 'approved'} personal={profile.company.entity_type === 'Perorangan'} />
		</section>
	</div>

	{#if profile.notes.length > 0}
		<section class="mt-6 rounded-xl border border-[#F3C1C1] bg-[#FDF4F4] p-5 shadow-sm">
			<h2 class="mb-3 text-base font-bold text-[#B02424]">{t('ky.st.reviewer')}</h2>
			<ul class="space-y-2">
				{#each profile.notes as n (n.id)}
					{#if !n.resolved}
						<li class="text-[13px] leading-relaxed text-[#6B3A3A]">
							{#if n.field}<b>{n.field}: </b>{/if}{n.note}
						</li>
					{/if}
				{/each}
			</ul>
		</section>
	{/if}
</div>
