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
	const doneN = $derived(check.filter((c) => c.complete).length);
	const pct = $derived(check.length ? (doneN / check.length) * 100 : 0);
	const flagCount = $derived(check.filter((c) => c.needs_revision).length);

	// Akun terverifikasi penuh: data disamarkan (blur) + pesan "DATA ANDA SUDAH VALID".
	const valid = $derived(profile.status === 'approved' && !profile.recheck);
	let reveal = $state(false);

	async function doSubmit() {
		submitting = true;
		summary = '';
		const r = await api.submit();
		if (!r.ok) summary = r.message ?? 'Gagal mengirim.';
		submitting = false;
	}
</script>

<div class="w-full">
	<!-- header status -->
	<div class="relative mb-5 overflow-hidden rounded-2xl border border-merah-gelap/10 bg-white p-5 shadow-sm">
		<div class="pointer-events-none absolute -right-10 -top-12 size-40 rounded-full bg-emas/20 blur-2xl"></div>
		<div class="relative flex flex-wrap items-center justify-between gap-3">
			<div>
				<h1 class="font-display text-2xl font-extrabold text-merah-gelap">Data Diri</h1>
				<p class="mt-0.5 text-[13px] text-merah-gelap/60">Lengkapi 4 bagian untuk verifikasi penerima pembayaran.</p>
			</div>
			<KycStatusBadge status={profile.status} recheck={profile.recheck} />
		</div>
		<div class="relative mt-4 flex items-center gap-3">
			<div class="h-2 flex-1 overflow-hidden rounded-full bg-merah-gelap/10">
				<div
					class="h-full rounded-full bg-linear-to-r from-merah to-emas transition-all duration-700"
					style="width: {pct}%"
				></div>
			</div>
			<span class="text-xs font-bold text-merah-gelap/70">{doneN}/{check.length} bagian</span>
		</div>
	</div>

	{#if profile.recheck}
		<div class="mb-4 rounded-xl border border-amber-300 bg-amber-50 p-3 text-[13px] text-amber-900">
			Akun terverifikasi namun ada bagian yang diubah sehingga <b>perlu ditinjau ulang</b>.
			Transaksi tetap berjalan (kebijakan sistem) sambil menunggu persetujuan admin.
		</div>
	{/if}
	{#if flagCount > 0}
		<div class="mb-4 rounded-xl border border-red-200 bg-red-50 p-3 text-[13px] text-red-800">
			<b>Perlu Revisi.</b> Perbaiki bagian bertanda merah lalu tekan <b>Kirim untuk Verifikasi</b>.
		</div>
	{/if}

	<!-- tab bar -->
	<div class="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-4" role="tablist">
		{#each TABS as t, i (t.key)}
			<button
				type="button"
				role="tab"
				aria-selected={active === t.key}
				class="rounded-xl border px-3 py-2.5 text-left transition focus-visible:ring-4 focus-visible:ring-emas/60
					{active === t.key ? 'border-merah bg-white shadow-sm' : 'border-merah-gelap/10 bg-white/60 hover:bg-white'}"
				onclick={() => (active = t.key)}
			>
				<span class="flex items-center gap-2">
					<span
						class="grid size-6 shrink-0 place-items-center rounded-full text-[11px] font-bold
						{done(t.key) ? 'bg-emerald-600 text-white' : flagged(t.key) ? 'bg-red-600 text-white' : active === t.key ? 'bg-merah text-white' : 'bg-merah-gelap/10 text-merah-gelap/70'}"
					>
						{#if done(t.key)}
							<svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="3"><path d="m5 12 5 5L20 7" /></svg>
						{:else if flagged(t.key)}
							!
						{:else}
							{i + 1}
						{/if}
					</span>
					<span class="text-[13px] font-semibold {active === t.key ? 'text-merah-gelap' : 'text-merah-gelap/60'}">{t.label}</span>
				</span>
			</button>
		{/each}
	</div>

	<!-- kartu putih bagian aktif -->
	<div class="relative overflow-hidden rounded-2xl border border-merah-gelap/10 bg-white shadow-sm">
	<div
		class="p-5 transition-[filter] duration-700 sm:p-6 {valid && !reveal ? 'pointer-events-none select-none blur-md' : ''}"
		aria-hidden={valid && !reveal}
		inert={valid && !reveal}
	>
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

	{#if valid && !reveal}
		<div class="valid-overlay absolute inset-0 grid place-items-center bg-white/40 p-6 text-center" role="status">
			<div class="valid-pop">
				<svg viewBox="0 0 72 72" class="mx-auto size-20" fill="none" aria-hidden="true">
					<circle class="valid-ring" cx="36" cy="36" r="32" stroke="#10b981" stroke-width="4" />
					<path class="valid-tick" d="M22 37l10 10 18-21" stroke="#10b981" stroke-width="5" stroke-linecap="round" stroke-linejoin="round" />
				</svg>
				<h2 class="valid-text mt-4 font-display text-2xl font-extrabold tracking-wide sm:text-3xl">DATA ANDA SUDAH VALID</h2>
				<p class="mt-1 text-[13px] font-medium text-merah-gelap/70">Seluruh data diri telah diverifikasi dan disetujui.</p>
				<button
					type="button"
					class="mt-4 rounded-xl border border-merah-gelap/15 bg-white px-4 py-2 text-xs font-bold text-merah-gelap transition hover:bg-gading focus-visible:ring-4 focus-visible:ring-emas/60"
					onclick={() => (reveal = true)}
				>
					Tampilkan data
				</button>
			</div>
		</div>
	{/if}
	</div>
	{#if valid && reveal}
		<div class="mt-2 text-right">
			<button type="button" class="text-xs font-bold text-merah underline" onclick={() => (reveal = false)}>Samarkan data</button>
		</div>
	{/if}

	<!-- navigasi + kirim -->
	<div class="mt-5 flex flex-col-reverse items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
		<button
			type="button"
			disabled={idx === 0}
			class="h-10 rounded-xl border border-merah-gelap/15 bg-white px-5 text-[13px] font-semibold text-merah-gelap/80 transition hover:bg-gading focus-visible:ring-4 focus-visible:ring-emas/60 disabled:cursor-not-allowed disabled:opacity-40"
			onclick={() => (active = TABS[idx - 1].key)}
		>
			← Sebelumnya
		</button>
		<div class="flex items-center gap-3">
			{#if summary}
				<p class="text-[13px] font-medium text-red-700">{summary}</p>
			{/if}
			{#if active !== 'documents'}
				<button
					type="button"
					class="h-10 rounded-xl bg-merah px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-merah-tua focus-visible:ring-4 focus-visible:ring-emas/60"
					disabled={moving}
						onclick={next}
				>
					Selanjutnya →
				</button>
			{:else if showSubmit}
				<button
					type="button"
					disabled={submitting || !allDone}
					class="h-10 rounded-xl bg-emerald-700 px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-emerald-800 focus-visible:ring-4 focus-visible:ring-emas/60 disabled:cursor-not-allowed disabled:opacity-50"
					onclick={doSubmit}
				>
					{submitting ? 'Mengirim…' : 'Kirim untuk Verifikasi'}
				</button>
			{:else if !editable && profile.status === 'submitted'}
				<span class="text-[13px] text-merah-gelap/60">Sedang diverifikasi — data terkunci.</span>
			{/if}
		</div>
	</div>
</div>

<style>
	.valid-overlay { animation: valid-fade 0.6s ease both; }
	.valid-pop { animation: valid-pop 0.7s cubic-bezier(0.2, 1.3, 0.4, 1) 0.15s both; }
	.valid-ring { stroke-dasharray: 202; stroke-dashoffset: 202; animation: valid-draw 0.8s ease 0.3s forwards; }
	.valid-tick { stroke-dasharray: 52; stroke-dashoffset: 52; animation: valid-draw 0.5s ease 0.9s forwards; }
	.valid-text {
		background: linear-gradient(90deg, #047857 30%, #e8b84a 50%, #047857 70%);
		background-size: 200% 100%;
		-webkit-background-clip: text;
		background-clip: text;
		color: transparent;
		animation: valid-shine 3.5s linear 1.4s infinite;
	}
	@keyframes valid-fade { from { opacity: 0; } to { opacity: 1; } }
	@keyframes valid-pop { from { opacity: 0; transform: scale(0.6) translateY(12px); } to { opacity: 1; transform: none; } }
	@keyframes valid-draw { to { stroke-dashoffset: 0; } }
	@keyframes valid-shine { from { background-position: 100% 0; } to { background-position: -100% 0; } }
	@media (prefers-reduced-motion: reduce) {
		.valid-overlay, .valid-pop, .valid-ring, .valid-tick, .valid-text { animation: none; stroke-dashoffset: 0; }
		.valid-text { background: none; color: #047857; }
	}
</style>
