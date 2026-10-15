<script lang="ts">
	// Bagian 2 — Perwakilan Bisnis. NIK ditampilkan ter-mask; penuh hanya saat mode edit
	// (profil edit dari /v1/kyc/profile/edit). KTP diunggah sebagai dokumen pasangan.
	import TextField from './TextField.svelte';
	import SelectField from './SelectField.svelte';
	import PhoneField from './PhoneField.svelte';
	import FileCard from './FileCard.svelte';
	import ReviewNote from './ReviewNote.svelte';
	import type { Note, Representative } from '../../server/kyc';
	import type { SectionApi } from './types';

	let {
		data,
		errors = {},
		notes = [],
		locked = false,
		busy = false,
		ktp = null,
		ktpBack = null,
		api,
		register
	}: {
		data: Representative;
		errors?: Record<string, string>;
		notes?: Note[];
		locked?: boolean;
		busy?: boolean;
		ktp?: { name: string; size: number; url?: string; mime?: string } | null;
		ktpBack?: { name: string; size: number; url?: string; mime?: string } | null;
		api: SectionApi;
		/** Daftarkan fungsi simpan agar tombol "Selanjutnya" menyimpan dulu. */
		register?: (fn: () => Promise<{ ok: boolean; message?: string }>) => void;
	} = $props();

	const POSITIONS = [
		{ value: 'Pemilik', label: 'Pemilik' },
		{ value: 'Direktur', label: 'Direktur' },
		{ value: 'Komisaris', label: 'Komisaris' },
		{ value: 'Manajer', label: 'Manajer' }
	];
	const NATIONS = [
		{ value: 'Indonesia', label: 'Indonesia' },
		{ value: 'WNA', label: 'Warga Negara Asing' }
	];

	// id_number omitempty di Go = undefined; bind:value pada TextField akan crash (halaman 500) bila undefined.
	let form = $state({ ...data, id_number: data.id_number ?? '' });
	let saving = $state(false);
	let msg = $state('');
	let fieldErr = $state<Record<string, string>>({});
	const errs = $derived({ ...errors, ...fieldErr });
	let ktpBusy = $state(false);
	let ktpBackBusy = $state(false);
	let docErrors = $state<Record<string, string>>({});

	async function save() {
		saving = true;
		// NIK dikirim hanya saat diketik ulang (kosong = tidak berubah); server yang menyegel.
		const payload: Record<string, unknown> = {
			name: form.name,
			position: form.position,
			phone: form.phone,
			email: form.email,
			nationality: form.nationality
		};
		if (form.id_number) payload.id_number = form.id_number;
		const r = await api.save('representative', payload);
		fieldErr = r.fields ?? {};
		msg = r.ok ? '' : (r.message ?? 'Gagal menyimpan.');
		saving = false;
		return r;
	}
	$effect(() => {
		register?.(() => (locked ? Promise.resolve({ ok: true }) : save()));
	});

	async function uploadKtp(side: 'ktp' | 'ktp_back', file: File) {
		if (side === 'ktp') ktpBusy = true;
		else ktpBackBusy = true;
		const r = await api.uploadDoc(side, file);
		docErrors = { ...docErrors, [side]: r.ok ? '' : (r.message ?? 'Unggahan gagal.') };
		if (side === 'ktp') ktpBusy = false;
		else ktpBackBusy = false;
	}
</script>

<div class="space-y-5">
	<ReviewNote notes={notes} section="representative" />
	{#if locked}
		<p class="text-[13px] text-[#7A8894]">Bagian ini terkunci saat verifikasi berjalan.</p>
	{/if}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<TextField id="kyc-rep-name" label="Nama lengkap (sesuai KTP)" bind:value={form.name} error={errs.name} />
		<SelectField id="kyc-rep-position" label="Jabatan" options={POSITIONS} bind:value={form.position} error={errs.position} />
		<PhoneField id="kyc-rep-phone" label="Nomor telepon pribadi" bind:value={form.phone} error={errs.phone} />
		<TextField id="kyc-rep-email" label="Email pribadi" type="email" bind:value={form.email} error={errs.email} />
		<SelectField id="kyc-rep-nation" label="Kewarganegaraan" options={NATIONS} bind:value={form.nationality} error={errs.nationality} />
		{#if form.id_masked}
			<TextField id="kyc-rep-nik-masked" label="NIK perwakilan" value={form.id_masked} masked hint="tersimpan — tulis ulang hanya jika berubah" />
			<TextField id="kyc-rep-nik-new" label="NIK baru (opsional)" inputmode="numeric" bind:value={form.id_number} hint="16 digit; dibiarkan kosong = tidak berubah" error={errs.id_number} />
		{:else}
			<div class="sm:col-span-2">
				<TextField id="kyc-rep-nik" label="NIK perwakilan" inputmode="numeric" bind:value={form.id_number}
					hint="16 digit sesuai KTP; disegel (tidak tampil penuh setelah disimpan)" error={errs.id_number} />
			</div>
		{/if}
		<div class="grid grid-cols-1 gap-4 sm:col-span-2 sm:grid-cols-2">
			<FileCard
				label="KTP perwakilan — sisi depan"
				hint="PDF/JPG/PNG, maks 15 MB"
				file={ktp}
				uploading={ktpBusy}
				locked={locked}
				error={docErrors.ktp}
				maxBytes={15 * 1024 * 1024}
				onfile={(f) => uploadKtp('ktp', f)}
			/>
			<FileCard
				label="KTP perwakilan — sisi belakang"
				hint="PDF/JPG/PNG, maks 15 MB"
				file={ktpBack}
				uploading={ktpBackBusy}
				locked={locked}
				error={docErrors.ktp_back}
				maxBytes={15 * 1024 * 1024}
				onfile={(f) => uploadKtp('ktp_back', f)}
			/>
		</div>
	</div>
	{#if msg}<p class="text-[13px] font-medium text-[#D32F2F]" role="alert">{msg}</p>{/if}
	<button
		type="button"
		disabled={locked || saving || busy}
		class="h-10 rounded-md bg-[#D32F2F] px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-[#B02424] disabled:cursor-not-allowed disabled:opacity-50"
		onclick={save}
	>
		{saving ? 'Menyimpan…' : 'Simpan Perwakilan'}
	</button>
</div>
