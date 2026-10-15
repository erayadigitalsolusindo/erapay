<script lang="ts">
	// Bagian 1 — Perusahaan. Business ID dibuat server (read-only di UI).
	// Alamat: autocomplete wilayah mengisi kode pos s/d provinsi sekaligus.
	import TextField from './TextField.svelte';
	import SelectField from './SelectField.svelte';
	import TextareaField from './TextareaField.svelte';
	import PhoneField from './PhoneField.svelte';
	import ReviewNote from './ReviewNote.svelte';
	import type { Company, Note, Region } from '../../server/kyc';
	import type { SectionApi } from './types';

	let {
		data,
		errors = {},
		notes = [],
		locked = false,
		busy = false,
		api
	}: {
		data: Company;
		errors?: Record<string, string>;
		notes?: Note[];
		locked?: boolean;
		busy?: boolean;
		api: SectionApi;
	} = $props();

	const ENTITY = [
		{ value: 'perseroan_terbatas', label: 'Perseroan Terbatas (PT)' },
		{ value: 'cv', label: 'Commanditaire Vennootschap (CV)' },
		{ value: 'perseorangan', label: 'Perseorangan' },
		{ value: 'yayasan', label: 'Yayasan' },
		{ value: 'koperasi', label: 'Koperasi' }
	];

	let form = $state({ ...data });
	let saving = $state(false);
	let regionHits: Region[] = $state([]);
	let regionOpen = $state(false);

	let timer: ReturnType<typeof setTimeout>;
	$effect(() => {
		// Autocomplete wilayah: kueri dari kode pos (debounce 250 ms).
		const v = form.postal_code;
		if (!v || v.trim().length < 3) {
			regionHits = [];
			regionOpen = false;
			return;
		}
		timer = setTimeout(async () => {
			regionHits = await api.regions(v.trim());
			regionOpen = regionHits.length > 0;
		}, 250);
	});

	function pickRegion(r: Region) {
		form.postal_code = r.postal_code;
		form.kelurahan = r.kelurahan;
		form.kecamatan = r.kecamatan;
		form.kota = r.kota;
		form.provinsi = r.provinsi;
		regionOpen = false;
	}

	async function save() {
		saving = true;
		const payload = {
			entity_type: form.entity_type,
			company_name: form.company_name,
			phone: form.phone,
			email: form.email,
			postal_code: form.postal_code,
			kelurahan: form.kelurahan,
			kecamatan: form.kecamatan,
			kota: form.kota,
			provinsi: form.provinsi,
			address_detail: form.address_detail
		};
		await api.save('company', payload);
		saving = false;
	}
</script>

<div class="space-y-5">
	<ReviewNote notes={notes} section="company" />
	{#if locked}
		<p class="text-[13px] text-[#7A8894]">Bagian ini terkunci saat verifikasi berjalan.</p>
	{/if}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<TextField id="kyc-biz-id" label="Business ID" value={form.business_id} hint="dibuat sistem" />
		<SelectField id="kyc-entity" label="Jenis entitas" options={ENTITY} bind:value={form.entity_type} error={errors.entity_type} />
		<TextField id="kyc-company-name" label="Nama badan usaha" bind:value={form.company_name} hint="sesuai akta" error={errors.company_name} />
		<PhoneField id="kyc-company-phone" label="Nomor telepon perusahaan" bind:value={form.phone} error={errors.phone} />
		<TextField id="kyc-company-email" label="Email perusahaan" type="email" bind:value={form.email} error={errors.email} />
		<div class="relative">
			<TextField id="kyc-postal" label="Kode pos" inputmode="numeric" bind:value={form.postal_code} error={errors.postal_code}
				hint="tulis kode pos / kelurahan, lalu pilih dari daftar" />
			{#if regionOpen && regionHits.length > 0}
				<ul class="absolute z-20 mt-1 max-h-56 w-full overflow-auto rounded-md border border-[#CBD2D9] bg-white py-1 shadow-lg">
					{#each regionHits as r, i (i)}
						<li>
							<button
								type="button"
								class="w-full px-3 py-2 text-left text-[13px] hover:bg-[#F4F7FA]"
								onclick={() => {
									pickRegion(r);
									regionOpen = false;
								}}
							>
								<span class="font-semibold text-[#1A2530]">{r.postal_code}</span>
								<span class="text-[#42505C]"> — {r.kelurahan}, {r.kecamatan}, {r.kota}, {r.provinsi}</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
		<TextField id="kyc-kelurahan" label="Kelurahan / Desa" bind:value={form.kelurahan} error={errors.kelurahan} />
		<TextField id="kyc-kecamatan" label="Kecamatan" bind:value={form.kecamatan} error={errors.kecamatan} />
		<TextField id="kyc-kota" label="Kota / Kabupaten" bind:value={form.kota} error={errors.kota} />
		<TextField id="kyc-provinsi" label="Provinsi" bind:value={form.provinsi} error={errors.provinsi} />
		<div class="sm:col-span-2">
			<TextareaField id="kyc-addr-detail" label="Alamat detail (jalan, nomor, RT/RW)" bind:value={form.address_detail} error={errors.address_detail} />
		</div>
	</div>
	<button
		type="button"
		disabled={locked || saving || busy}
		class="h-10 rounded-md bg-[#D32F2F] px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-[#B02424] disabled:cursor-not-allowed disabled:opacity-50"
		onclick={save}
	>
		{saving ? 'Menyimpan…' : 'Simpan Perusahaan'}
	</button>
</div>
