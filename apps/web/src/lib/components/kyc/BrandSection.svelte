<script lang="ts">
	// Bagian 3 — Merek. Brand ID dibuat server. Logo & bukti diunggah langsung
	// (multipart), tautan jualan disimpan bersama bagian ini.
	import TextField from './TextField.svelte';
	import SelectField from './SelectField.svelte';
	import TextareaField from './TextareaField.svelte';
	import FileCard from './FileCard.svelte';
	import ImageGallery from './ImageGallery.svelte';
	import LinkList from './LinkList.svelte';
	import ReviewNote from './ReviewNote.svelte';
	import type { Brand, Note } from '../../server/kyc';
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
		data: Brand;
		errors?: Record<string, string>;
		notes?: Note[];
		locked?: boolean;
		busy?: boolean;
		api: SectionApi;
		/** Daftarkan fungsi simpan agar tombol "Selanjutnya" menyimpan dulu. */
		register?: (fn: () => Promise<{ ok: boolean; message?: string }>) => void;
	} = $props();

	const CATEGORIES = [
		{ value: 'retail', label: 'Ritel / Toko' },
		{ value: 'kuliner', label: 'Kuliner' },
		{ value: 'jasa', label: 'Jasa' },
		{ value: 'digital', label: 'Produk Digital' },
		{ value: 'lainnya', label: 'Lainnya' }
	];

	let form = $state({ ...data });
	let links: string[] = $state((data.links ?? []).map((l) => l.url));
	let amount = $state(data.monthly_amount ? String(data.monthly_amount) : '');
	let count = $state(data.monthly_count ? String(data.monthly_count) : '');
	let saving = $state(false);
	let msg = $state('');
	let fieldErr = $state<Record<string, string>>({});
	const errs = $derived({ ...errors, ...fieldErr });
	let logoBusy = $state(false);
	let evBusy = $state(false);
	let uploadErrors = $state<Record<string, string>>({});

	async function save() {
		saving = true;
		const r = await api.save('brand', {
			name: form.name,
			category: form.category,
			description: form.description,
			monthly_amount: Number(amount.replace(/\D/g, '')) || 0,
			monthly_count: Number(count.replace(/\D/g, '')) || 0,
			links: links.map((l) => l.trim()).filter(Boolean)
		});
		fieldErr = r.fields ?? {};
		msg = r.ok ? '' : (r.message ?? 'Gagal menyimpan.');
		saving = false;
		return r;
	}
	$effect(() => {
		register?.(() => (locked ? Promise.resolve({ ok: true }) : save()));
	});

	async function onLogo(file: File) {
		logoBusy = true;
		const r = await api.uploadLogo(file);
		if (!r.ok) uploadErrors = { ...uploadErrors, logo: r.message ?? 'Unggahan gagal.' };
		logoBusy = false;
	}
	async function onAddEvidence(file: File) {
		evBusy = true;
		const r = await api.addEvidence(file);
		if (!r.ok) uploadErrors = { ...uploadErrors, evidence: r.message ?? 'Unggahan gagal.' };
		evBusy = false;
	}
	async function onDeleteEvidence(id: string) {
		await api.deleteEvidence(id);
	}
</script>

<div class="space-y-5">
	<ReviewNote notes={notes} section="brand" />
	{#if locked}
		<p class="text-[13px] text-[#7A8894]">Bagian ini terkunci saat verifikasi berjalan.</p>
	{/if}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<TextField id="kyc-brand-id" label="Brand ID" value={form.brand_id} hint="dibuat sistem" />
		<TextField id="kyc-brand-name" label="Nama merek" bind:value={form.name} error={errs.name} />
		<SelectField id="kyc-brand-cat" label="Kategori" options={CATEGORIES} bind:value={form.category} error={errs.category} />
		<div class="grid grid-cols-2 gap-4 sm:col-span-2">
			<TextField
				id="kyc-brand-amount"
				label="Perkiraan nilai transaksi / bulan (Rp)"
				inputmode="numeric"
				bind:value={amount}
				error={errs.monthly_amount}
			/>
			<TextField
				id="kyc-brand-count"
				label="Perkiraan jumlah transaksi / bulan"
				inputmode="numeric"
				bind:value={count}
				error={errs.monthly_count}
			/>
		</div>
		<div class="sm:col-span-2">
			<TextareaField id="kyc-brand-desc" label="Deskripsi singkat usaha" bind:value={form.description} error={errs.description} />
		</div>
		<div>
			<FileCard
				label="Logo merek"
				accept=".jpg,.jpeg,.png"
				hint="JPG/PNG, maks 5 MB — tampil 72×72 px"
				maxBytes={5 * 1024 * 1024}
				file={data.logo ? { name: data.logo.name, size: data.logo.size, url: data.logo.url, mime: data.logo.mime } : null}
				uploading={logoBusy}
				locked={locked}
				error={uploadErrors.logo}
				onfile={onLogo}
			/>
		</div>
		<div class="sm:col-span-2">
			<ImageGallery evidences={data.evidences ?? []} busy={evBusy} {locked} error={uploadErrors.evidence} onadd={onAddEvidence} ondelete={onDeleteEvidence} />
		</div>
		<div class="sm:col-span-2">
			<LinkList {links} {locked} error={errs.links} onchange={(v) => (links = v)} />
		</div>
	</div>
	{#if msg}<p class="text-[13px] font-medium text-[#D32F2F]" role="alert">{msg}</p>{/if}
	<button
		type="button"
		disabled={locked || saving || busy}
		class="h-10 rounded-md bg-[#D32F2F] px-5 text-[13px] font-bold tracking-wide text-white transition hover:bg-[#B02424] disabled:cursor-not-allowed disabled:opacity-50"
		onclick={save}
	>
		{saving ? 'Menyimpan…' : 'Simpan Merek'}
	</button>
</div>
