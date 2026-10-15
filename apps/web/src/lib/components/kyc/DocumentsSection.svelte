<script lang="ts">
	// Bagian 4 — Dokumen legal. Satu jenis = satu berkas (unggah ulang menimpa).
	// NPWP Badan + nomor, Akta (25 MB), SK Kemenkumham, NIB + nomor + KBLI 5 digit,
	// Surat Izin Usaha. Saat merchant sudah approved, mengganti dokumen legal memicu
	// peninjauan ulang (NEEDS_REVIEW_AGAIN): server menuntut konfirmasi eksplisit.
	import { getI18n } from '../../i18n/i18n.svelte';
	import TextField from './TextField.svelte';
	import FileCard from './FileCard.svelte';
	import ReviewNote from './ReviewNote.svelte';
	import type { DocKind, Note } from '../../server/kyc';
	import type { SectionApi } from './types';

	let {
		documents,
		errors = {},
		notes = [],
		locked = false,
		busy = false,
		approved = false,
		personal = false,
		api
	}: {
		documents: Record<DocKind, { name: string; size: number } | null>;
		errors?: Record<string, string>;
		notes?: Note[];
		locked?: boolean;
		busy?: boolean;
		approved?: boolean;
		/** Entitas perorangan: dokumen badan usaha tidak perlu diisi. */
		personal?: boolean;
		api: SectionApi;
	} = $props();

	const { t, tx } = getI18n();
	let npwpNumber = $state('');
	let nibNumber = $state('');
	let kbli = $state('');
	let confirm = $state(false);
	let busyDoc: DocKind | null = $state(null);
	let docErr = $state<Record<string, string>>({});

	const err = (k: string) => docErr[k] ?? errors[k] ?? '';

	async function upload(docType: DocKind, file: File) {
		busyDoc = docType;
		const extra: { number?: string; kbli?: string; confirm?: boolean } = {};
		if (docType === 'npwp_badan') extra.number = npwpNumber;
		if (docType === 'nib') {
			extra.number = nibNumber;
			extra.kbli = kbli;
		}
		if (approved) extra.confirm = confirm;
		const r = await api.uploadDoc(docType, file, extra);
		if (!r.ok) {
			const next = { ...docErr };
			if (r.fields) for (const [k, v] of Object.entries(r.fields)) next[k] = v;
			else next[docType] = r.message ?? t('ky.uploadFail');
			docErr = next;
		}
		busyDoc = null;
	}
</script>

<div class="space-y-5">
	<ReviewNote notes={notes} section="documents" />
	{#if locked}
		<p class="text-[13px] text-[#7A8894]">{t('ky.docsLocked')}</p>
	{/if}
	<div class="relative">
		<div
			class="transition {personal ? 'pointer-events-none select-none opacity-60 blur-[2px]' : ''}"
			inert={personal}
			aria-hidden={personal}
		>
		<div class="grid grid-cols-1 gap-5 md:grid-cols-2">
			<div class="space-y-3">
				<FileCard
					label={t('ky.doc.npwp')}
					hint={t('ky.file.hint15')}
					file={documents.npwp_badan}
					uploading={busyDoc === 'npwp_badan'}
					locked={locked}
					error={err('npwp_badan')}
					onfile={(f) => upload('npwp_badan', f)}
				/>
				<TextField id="kyc-npwp-num" label={t('ky.doc.npwpNo')} inputmode="numeric" bind:value={npwpNumber}
					hint={t('ky.doc.npwpHint')} error={err('npwp_number')} />
			</div>
			<div class="space-y-3">
				<FileCard
					label={t('ky.doc.akta')}
					hint={t('ky.file.hint25')}
					maxBytes={25 * 1024 * 1024}
					file={documents.akta}
					uploading={busyDoc === 'akta'}
					locked={locked}
					error={err('akta')}
					onfile={(f) => upload('akta', f)}
				/>
			</div>
			<div class="space-y-3">
				<FileCard
					label={t('ky.doc.sk')}
					hint={t('ky.doc.skHint')}
					file={documents.sk_kemenkumham}
					uploading={busyDoc === 'sk_kemenkumham'}
					locked={locked}
					error={err('sk_kemenkumham')}
					onfile={(f) => upload('sk_kemenkumham', f)}
				/>
			</div>
			<div class="space-y-3">
				<FileCard
					label={t('ky.doc.izin')}
					hint={t('ky.doc.izinHint')}
					file={documents.izin_usaha}
					uploading={busyDoc === 'izin_usaha'}
					locked={locked}
					error={err('izin_usaha')}
					onfile={(f) => upload('izin_usaha', f)}
				/>
			</div>
			<div class="space-y-3 md:col-span-2">
				<FileCard
					label={t('ky.doc.nib')}
					hint={t('ky.file.hint15')}
					file={documents.nib}
					uploading={busyDoc === 'nib'}
					locked={locked}
					error={err('nib')}
					onfile={(f) => upload('nib', f)}
				/>
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<TextField id="kyc-nib-num" label={t('ky.doc.nibNo')} inputmode="numeric" bind:value={nibNumber} hint={t('ky.doc.nibHint')} error={err('nib_number')} />
					<TextField id="kyc-kbli" label={t('ky.doc.kbli')} inputmode="numeric" bind:value={kbli} hint={t('ky.doc.kbliHint')} error={err('kbli')} />
				</div>
			</div>
		</div>
		</div>
		{#if personal}
			<div class="pointer-events-none absolute inset-0 grid place-items-center">
				<span class="-rotate-12 select-none text-center text-3xl font-extrabold italic tracking-widest text-[#D32F2F]/50 sm:text-5xl">
					{t('ky.doc.notNeeded')}
				</span>
			</div>
		{/if}
	</div>
	{#if approved && !personal}
		<label
			class="flex cursor-pointer items-start gap-2.5 rounded-md border border-[#F3C1C1] bg-[#FDF4F4] p-3"
		>
			<input type="checkbox" bind:checked={confirm} class="mt-0.5 accent-[#D32F2F]" />
			<span class="text-[13px] leading-relaxed text-[#6B3A3A]">{t('ky.doc.confirm')}</span>
		</label>
	{/if}
</div>
