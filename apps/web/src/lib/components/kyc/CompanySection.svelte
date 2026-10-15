<script lang="ts">
	// Bagian 1 — Perusahaan. Business ID dibuat server (read-only di UI).
	// Alamat: autocomplete wilayah mengisi kode pos s/d provinsi sekaligus.
	import { getI18n } from '../../i18n/i18n.svelte';
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
		api,
		register
	}: {
		data: Company;
		errors?: Record<string, string>;
		notes?: Note[];
		locked?: boolean;
		busy?: boolean;
		api: SectionApi;
		/** Daftarkan fungsi simpan agar tombol "Selanjutnya" menyimpan dulu. */
		register?: (fn: () => Promise<{ ok: boolean; message?: string }>) => void;
	} = $props();

	const { t, tx } = getI18n();
	const ENTITY = $derived(['PT', 'CV', 'Yayasan', 'Perorangan'].map((v) => ({ value: v, label: t(`ky.ent.${v}` as 'ky.ent.PT') })));

	let form = $state({ ...data });
	let saving = $state(false);
	let msg = $state('');
	let fieldErr = $state<Record<string, string>>({});
	const errs = $derived({ ...errors, ...fieldErr });
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
		const r = await api.save('company', payload);
		fieldErr = r.fields ?? {};
		msg = r.ok ? '' : (r.message ?? t('ky.saveFail'));
		saving = false;
		return r;
	}
	$effect(() => {
		register?.(() => (locked ? Promise.resolve({ ok: true }) : save()));
	});
</script>

<div class="space-y-5">
	<ReviewNote notes={notes} section="company" />
	{#if locked}
		<p class="text-[13px] text-[#7A8894]">{t('ky.locked')}</p>
	{/if}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<TextField id="kyc-biz-id" label="Business ID" value={form.business_id} hint={t('ky.co.bizIdHint')} readonly />
		<SelectField id="kyc-entity" label={t('ky.co.entity')} options={ENTITY} bind:value={form.entity_type} error={errs.entity_type} />
		<TextField id="kyc-company-name" label={t('ky.co.name')} bind:value={form.company_name} hint={t('ky.co.nameHint')} error={errs.company_name} />
		<PhoneField id="kyc-company-phone" label={t('ky.co.phone')} bind:value={form.phone} error={errs.phone} />
		<TextField id="kyc-company-email" label={t('ky.co.email')} type="email" bind:value={form.email} error={errs.email} />
		<div class="relative">
			<TextField id="kyc-postal" label={t('ky.co.postal')} inputmode="numeric" bind:value={form.postal_code} error={errs.postal_code}
				hint={t('ky.co.postalHint')} />
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
		<TextField id="kyc-kelurahan" label={t('ky.co.kelurahan')} bind:value={form.kelurahan} error={errs.kelurahan} />
		<TextField id="kyc-kecamatan" label={t('ky.co.kecamatan')} bind:value={form.kecamatan} error={errs.kecamatan} />
		<TextField id="kyc-kota" label={t('ky.co.kota')} bind:value={form.kota} error={errs.kota} />
		<TextField id="kyc-provinsi" label={t('ky.co.provinsi')} bind:value={form.provinsi} error={errs.provinsi} />
		<div class="sm:col-span-2">
			<TextareaField id="kyc-addr-detail" label={t('ky.co.addr')} bind:value={form.address_detail} error={errs.address_detail} />
		</div>
	</div>
	{#if msg}<p class="text-[13px] font-medium text-[#D32F2F]" role="alert">{msg}</p>{/if}
	<button
		type="button"
		disabled={locked || saving || busy}
		class="h-10 rounded-md bg-[#D32F2F] px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-[#B02424] disabled:cursor-not-allowed disabled:opacity-50"
		onclick={save}
	>
		{saving ? t('ky.saving') : t('ky.co.save')}
	</button>
</div>
