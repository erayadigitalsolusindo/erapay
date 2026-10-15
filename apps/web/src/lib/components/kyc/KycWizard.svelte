<script lang="ts">
	// Wizard "Data Diri" (4 bagian, gaya DOKU): Perusahaan, Perwakilan, Merek, Dokumen.
	// Dipakai halaman onboarding / Status Akun. Semua aturan ada di server (lib/server/kyc.ts).
	import KycStatusBadge from './KycStatusBadge.svelte';
	import CompanySection from './CompanySection.svelte';
	import RepresentativeSection from './RepresentativeSection.svelte';
	import BrandSection from './BrandSection.svelte';
	import DocumentsSection from './DocumentsSection.svelte';
	import type { Profile } from '../../server/kyc';
	import type { SectionApi } from './types';

	let { profile, api, ctaHref = '/dashboard/onboarding' }: { profile: Profile; api: SectionApi; ctaHref?: string } = $props();

	const TABS = [
		{ key: 'company', label: 'Perusahaan' },
		{ key: 'representative', label: 'Perwakilan' },
		{ key: 'brand', label: 'Merek' },
		{ key: 'documents', label: 'Dokumen' }
	] as const;

	let active = $state<string>('company');
	let submitting = $state(false);
	let sectionErr = $state<Record<string, Record<string, string>>>({});
	let summary = $state('');
	let idx = $derived(TABS.findIndex((t) => t.key === active));
	let saveActive: (() => Promise<{ ok: boolean; message?: string }>) | null = null;
	let moving = $state(false);
	const register = (fn: () => Promise<{ ok: boolean; message?: string }>) => (saveActive = fn);

	// Simpan bagian aktif lebih dulu; pindah tab hanya bila berhasil (data tidak hilang saat refresh).
	async function next() {
		moving = true;
		summary = '';
		const r = saveActive ? await saveActive() : { ok: true };
		moving = false;
		if (r.ok) active = TABS[idx + 1].key;
	}

	const check = $derived(profile.checklist ?? []);
	const done = (s: string) => check.find((c) => c.section === s)?.complete ?? false;
	const flagged = (s: string) => check.find((c) => c.section === s)?.needs_revision ?? false;

	const editable = $derived(profile.editable && !profile.locked);
	// status rejected = penolakan, tapi merchant boleh memperbaiki & mengajukan ulang (keputusan 15 Okt 2026)
	const showSubmit = $derived(profile.status === 'draft' || profile.status === 'action_required' || profile.status === 'rejected');
	const allDone = $derived(check.length > 0 && check.every((c) => c.complete));
	const flagCount = $derived(check.filter((c) => c.needs_revision).length);

	async function doSubmit() {
		submitting = true;
		summary = '';
		const r = await api.submit();
		if (!r.ok) summary = r.message ?? 'Gagal mengirim.';
		submitting = false;
	}
</script>

<div class="w-full">
	<!-- header status + CTA -->
	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<div>
			<h1 class="font-display text-xl font-bold text-[#1A2530]">Data Diri</h1>
			<p class="text-[13px] text-[#7A8894]">Lengkapi 4 bagian untuk verifikasi penerima pembayaran.</p>
		</div>
		<KycStatusBadge status={profile.status} recheck={profile.recheck} />
	</div>

	{#if profile.recheck}
		<div class="mb-4 rounded-md bg-[#FFF4E0] p-3 text-[13px] text-[#8A5A00]">
			Akun terverifikasi namun ada bagian yang diubah sehingga <b>perlu ditinjau ulang</b>.
			Transaksi tetap berjalan (kebijakan sistem) sambil menunggu persetujuan admin.
		</div>
	{/if}
	{#if flagCount > 0}
		<div class="mb-4 rounded-md bg-[#FDF4F4] p-3 text-[13px] text-[#6B3A3A]">
			<b>Perlu Revisi.</b> Perbaiki bagian bertanda merah lalu tekan <b>Kirim untuk Verifikasi</b>.
		</div>
	{/if}

	<!-- tab bar -->
	<div class="mb-5 grid grid-cols-2 gap-2 sm:grid-cols-4">
		{#each TABS as t, i (t.key)}
			<button
				type="button"
				class="relative rounded-md border px-3 py-2.5 text-left transition
					{active === t.key ? 'border-[#D32F2F] bg-white shadow-sm' : 'border-[#E5E8ED] bg-[#EFF2F5]'}"
				onclick={() => (active = t.key)}
			>
				<span class="flex items-center gap-2">
					<span
						class="grid size-6 shrink-0 place-items-center rounded-full text-[11px] font-bold
						{done(t.key) ? 'bg-[#1D7A46] text-white' : flagged(t.key) ? 'bg-[#D32F2F] text-white' : active === t.key ? 'bg-[#D32F2F] text-white' : 'bg-[#CBD2D9] text-[#42505C]'}"
					>
						{#if done(t.key)}
							<svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="3"><path d="m5 12 5 5L20 7" /></svg>
						{:else if flagged(t.key)}
							!
						{:else}
							{i + 1}
						{/if}
					</span>
					<span class="text-[13px] font-semibold {active === t.key ? 'text-[#1A2530]' : 'text-[#42505C]'}">{t.label}</span>
				</span>
			</button>
		{/each}
	</div>
	<!-- progress -->
	<div class="mb-6 h-1.5 w-full overflow-hidden rounded-full bg-[#E5E8ED]">
		<div
			class="h-full rounded-full bg-[#D32F2F] transition-all"
			style="width: {check.length ? (check.filter((c) => c.complete).length / check.length) * 100 : 0}%"
		></div>
	</div>

	<!-- kartu putih bagian aktif -->
	<div class="rounded-xl border border-[#E5E8ED] bg-white p-5 shadow-sm sm:p-6">
		{#if active === 'company'}
			<CompanySection data={profile.company} errors={sectionErr.company ?? {}} notes={profile.notes} locked={!editable} api={api} {register} />
		{:else if active === 'representative'}
			<RepresentativeSection data={profile.representative} errors={sectionErr.representative ?? {}} notes={profile.notes}
				ktp={profile.representative.ktp} ktpBack={profile.representative.ktp_back} locked={!editable} api={api} {register} />
		{:else if active === 'brand'}
			<BrandSection data={profile.brand} errors={sectionErr.brand ?? {}} notes={profile.notes} locked={!editable} api={api} {register} />
		{:else if active === 'documents'}
			<DocumentsSection documents={profile.documents} errors={sectionErr.documents ?? {}} notes={profile.notes}
				locked={!editable} approved={profile.status === 'approved'} personal={profile.company.entity_type === 'Perorangan'} api={api} />
		{/if}
	</div>

	<!-- navigasi + kirim -->
	<div class="mt-5 flex flex-col-reverse items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
		<button
			type="button"
			disabled={idx === 0}
			class="h-10 rounded-md border border-[#CBD2D9] bg-white px-5 text-[13px] font-semibold text-[#42505C] transition hover:border-[#7A8894] disabled:cursor-not-allowed disabled:opacity-40"
			onclick={() => (active = TABS[idx - 1].key)}
		>
			← Sebelumnya
		</button>
		<div class="flex items-center gap-3">
			{#if summary}
				<p class="text-[13px] font-medium text-[#D32F2F]">{summary}</p>
			{/if}
			{#if active !== 'documents'}
				<button
					type="button"
					class="h-10 rounded-md bg-[#D32F2F] px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-[#B02424]"
					disabled={moving}
						onclick={next}
				>
					Selanjutnya →
				</button>
			{:else if showSubmit}
				<button
					type="button"
					disabled={submitting || !allDone}
					class="h-10 rounded-md bg-[#1D7A46] px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-[#166438] disabled:cursor-not-allowed disabled:opacity-50"
					onclick={doSubmit}
				>
					{submitting ? 'Mengirim…' : 'Kirim untuk Verifikasi'}
				</button>
			{:else if !editable && profile.status === 'submitted'}
				<span class="text-[13px] text-[#7A8894]">Sedang diverifikasi — data terkunci.</span>
			{/if}
		</div>
	</div>
</div>
