<script lang="ts">
	// Bagian 2 — Perwakilan Bisnis. NIK ditampilkan ter-mask; penuh hanya saat mode edit
	// (profil edit dari /v1/kyc/profile/edit). KTP diunggah sebagai dokumen pasangan.
	import { getI18n } from '../../i18n/i18n.svelte';
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

	const { t, tx } = getI18n();
	const POSITIONS = $derived(['Pemilik', 'Direktur', 'Komisaris', 'Manajer'].map((v) => ({ value: v, label: t(`ky.pos.${v}` as 'ky.pos.Pemilik') })));
	const NATIONS = $derived(['Indonesia', 'WNA'].map((v) => ({ value: v, label: t(`ky.nat.${v}` as 'ky.nat.WNA') })));

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
		msg = r.ok ? '' : (r.message ?? t('ky.saveFail'));
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
		docErrors = { ...docErrors, [side]: r.ok ? '' : (r.message ?? t('ky.uploadFail')) };
		if (side === 'ktp') ktpBusy = false;
		else ktpBackBusy = false;
	}
</script>

<div class="space-y-5">
	<ReviewNote notes={notes} section="representative" />
	{#if locked}
		<p class="text-[13px] text-[#7A8894]">{t('ky.locked')}</p>
	{/if}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<TextField id="kyc-rep-name" label={t('ky.rep.name')} bind:value={form.name} error={errs.name} />
		<SelectField id="kyc-rep-position" label={t('ky.rep.position')} options={POSITIONS} bind:value={form.position} error={errs.position} />
		<PhoneField id="kyc-rep-phone" label={t('ky.rep.phone')} bind:value={form.phone} error={errs.phone} />
		<TextField id="kyc-rep-email" label={t('ky.rep.email')} type="email" bind:value={form.email} error={errs.email} />
		<SelectField id="kyc-rep-nation" label={t('ky.rep.nation')} options={NATIONS} bind:value={form.nationality} error={errs.nationality} />
		{#if form.id_masked}
			<TextField id="kyc-rep-nik-masked" label={t('ky.rep.nik')} value={form.id_masked} masked hint={t('ky.rep.nikMaskedHint')} />
			<TextField id="kyc-rep-nik-new" label={t('ky.rep.nikNew')} inputmode="numeric" bind:value={form.id_number} hint={t('ky.rep.nikNewHint')} error={errs.id_number} />
		{:else}
			<div class="sm:col-span-2">
				<TextField id="kyc-rep-nik" label={t('ky.rep.nik')} inputmode="numeric" bind:value={form.id_number}
					hint={t('ky.rep.nikHint')} error={errs.id_number} />
			</div>
		{/if}
		<div class="grid grid-cols-1 gap-4 sm:col-span-2 sm:grid-cols-2">
			<FileCard
				label={t('ky.rep.ktpFront')}
				hint={t('ky.file.hint15')}
				file={ktp}
				uploading={ktpBusy}
				locked={locked}
				error={docErrors.ktp}
				maxBytes={15 * 1024 * 1024}
				onfile={(f) => uploadKtp('ktp', f)}
			/>
			<FileCard
				label={t('ky.rep.ktpBack')}
				hint={t('ky.file.hint15')}
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
		{saving ? t('ky.saving') : t('ky.rep.save')}
	</button>
</div>
