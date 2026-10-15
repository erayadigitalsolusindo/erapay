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
		api
	}: {
		data: Representative;
		errors?: Record<string, string>;
		notes?: Note[];
		locked?: boolean;
		busy?: boolean;
		ktp?: { name: string; size: number } | null;
		api: SectionApi;
	} = $props();

	const POSITIONS = [
		{ value: 'pemilik', label: 'Pemilik' },
		{ value: 'direktur', label: 'Direktur' },
		{ value: 'komisaris', label: 'Komisaris' },
		{ value: 'pimpinan', label: 'Pimpinan' }
	];
	const NATIONS = [
		{ value: 'indonesia', label: 'Indonesia' },
		{ value: 'asing', label: 'Warga Negara Asing' }
	];

	let form = $state({ ...data });
	let saving = $state(false);
	let ktpBusy = $state(false);
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
		await api.save('representative', payload);
		saving = false;
	}

	async function uploadKtp(file: File) {
		ktpBusy = true;
		const r = await api.uploadDoc('ktp', file);
		if (!r.ok) docErrors = { ...docErrors, ktp: r.message ?? 'Unggahan gagal.' };
		ktpBusy = false;
	}
</script>

<div class="space-y-5">
	<ReviewNote notes={notes} section="representative" />
	{#if locked}
		<p class="text-[13px] text-[#7A8894]">Bagian ini terkunci saat verifikasi berjalan.</p>
	{/if}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<TextField id="kyc-rep-name" label="Nama lengkap (sesuai KTP)" bind:value={form.name} error={errors.name} />
		<SelectField id="kyc-rep-position" label="Jabatan" options={POSITIONS} bind:value={form.position} error={errors.position} />
		<PhoneField id="kyc-rep-phone" label="Nomor telepon pribadi" bind:value={form.phone} error={errors.phone} />
		<TextField id="kyc-rep-email" label="Email pribadi" type="email" bind:value={form.email} error={errors.email} />
		<SelectField id="kyc-rep-nation" label="Kewarganegaraan" options={NATIONS} bind:value={form.nationality} error={errors.nationality} />
		{#if form.id_masked}
			<TextField id="kyc-rep-nik-masked" label="NIK perwakilan" value={form.id_masked} masked hint="tersimpan — tulis ulang hanya jika berubah" />
			<TextField id="kyc-rep-nik-new" label="NIK baru (opsional)" inputmode="numeric" bind:value={form.id_number} hint="16 digit; dibiarkan kosong = tidak berubah" error={errors.id_number} />
		{:else}
			<div class="sm:col-span-2">
				<TextField id="kyc-rep-nik" label="NIK perwakilan" inputmode="numeric" bind:value={form.id_number}
					hint="16 digit sesuai KTP; disegel (tidak tampil penuh setelah disimpan)" error={errors.id_number} />
			</div>
		{/if}
		<div class="sm:col-span-2">
			<FileCard
				label="Kartu Tanda Penduduk (KTP) perwakilan"
				hint="PDF/JPG/PNG, maks 15 MB"
				file={ktp}
				uploading={ktpBusy}
				locked={locked}
				error={docErrors.ktp}
				maxBytes={15 * 1024 * 1024}
				onfile={uploadKtp}
			/>
		</div>
	</div>
	<button
		type="button"
		disabled={locked || saving || busy}
		class="h-10 rounded-md bg-[#D32F2F] px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-[#B02424] disabled:cursor-not-allowed disabled:opacity-50"
		onclick={save}
	>
		{saving ? 'Menyimpan…' : 'Simpan Perwakilan'}
	</button>
</div>
