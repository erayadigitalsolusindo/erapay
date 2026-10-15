<script lang="ts">
	import { getI18n } from '../../i18n/i18n.svelte';
	import ImageLightbox from './ImageLightbox.svelte';
	const { t } = getI18n();

	// Kartu unggah gaya DOKU: zona dashed, ringkasan berkas terunggah, tombol ganti.
	// Batas: PDF/JPG/PNG, 15 MB (Akta 25 MB). Pengecekan ulang tetap dilakukan server.
	let {
		label,
		hint = '',
		accept = '.pdf,.jpg,.jpeg,.png',
		maxBytes = 15 * 1024 * 1024,
		file = null,
		uploading = false,
		error = '',
		locked = false,
		onfile
	}: {
		label: string;
		hint?: string;
		accept?: string;
		maxBytes?: number;
		file?: { name: string; size: number; url?: string; mime?: string } | null;
		uploading?: boolean;
		error?: string;
		locked?: boolean;
		onfile: (f: File) => void;
	} = $props();

	let input = $state<HTMLInputElement>();
	let localErr = $state('');
	// Pratinjau gambar: object URL lokal sesaat setelah dipilih, lalu URL dari server.
	let localUrl = $state<string | null>(null);
	let zoom = $state(false);
	const isImg = (m?: string, n?: string) => !!m?.startsWith('image/') || /\.(jpe?g|png)$/i.test(n ?? '');
	const thumb = $derived(localUrl ?? (file?.url && isImg(file.mime, file.name) ? file.url : null));
	$effect(() => () => {
		if (localUrl) URL.revokeObjectURL(localUrl);
	});
	const finalErr = $derived(localErr || error);

	const fmtSize = (n: number) =>
		n >= 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MB` : `${Math.ceil(n / 1024)} KB`;

	function pick(f?: File) {
		if (!f) return;
		localErr = '';
		if (f.size > maxBytes) {
			localErr = t('ky.file.maxSize', { n: Math.round(maxBytes / 1024 / 1024) });
			return;
		}
		if (localUrl) URL.revokeObjectURL(localUrl);
		localUrl = isImg(f.type, f.name) ? URL.createObjectURL(f) : null;
		onfile(f);
	}
</script>

<div class="w-full">
	<label class="mb-1.5 block text-[13px] font-semibold text-[#1A2530]">{label}</label>
	{#if locked && !file}
		<div class="rounded-md border border-dashed border-[#CBD2D9] bg-[#F4F7FA] px-4 py-3 text-[13px] text-[#7A8894]">
			{t('ky.file.locked')}
		</div>
	{:else if file}
		<div class="overflow-hidden rounded-md border border-[#CBD2D9] bg-white">
			{#if thumb}
				<button
					type="button"
					class="block w-full cursor-zoom-in bg-[#F4F7FA]"
					aria-label={t('ky.file.zoom', { label })}
					onclick={() => (zoom = true)}
				>
					<img src={thumb} alt={t('ky.file.preview', { label })} class="h-32 w-full object-contain" />
				</button>
			{/if}
			<div class="flex items-center gap-3 px-3 py-2.5">
				{#if !thumb}
					<span class="grid size-9 shrink-0 place-items-center rounded bg-[#F4F7FA] text-[#D32F2F]">
						<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8">
							<path d="M14 3v5h5" /><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
						</svg>
					</span>
				{/if}
				<div class="min-w-0 flex-1">
					<p class="truncate text-[13px] font-semibold text-[#1A2530]">{file.name}</p>
					<p class="text-[12px] text-[#7A8894]">{fmtSize(file.size)}</p>
				</div>
				{#if uploading}
					<span class="text-[12px] font-medium text-[#7A8894]">{t('ky.uploading')}</span>
				{:else if !locked}
					<button
						type="button"
						class="rounded-md border border-[#CBD2D9] bg-white px-3 py-1.5 text-[12px] font-semibold text-[#D32F2F] transition hover:border-[#D32F2F] hover:text-[#B02424]"
						onclick={() => input?.click()}
					>
						{t('ky.file.replace')}
					</button>
				{/if}
			</div>
			<input
				bind:this={input}
				type="file"
				{accept}
				class="hidden"
				onchange={(e) => {
					const f = (e.currentTarget as HTMLInputElement).files?.[0];
					pick(f ?? undefined);
					if (input) input.value = '';
				}}
			/>
		</div>
	{:else}
		<button
			type="button"
			class="flex w-full flex-col items-center gap-1.5 rounded-md border-2 border-dashed px-4 py-6 text-center transition hover:border-[#D32F2F]/60 hover:bg-[#F4F7FA]
				{error || localErr ? 'border-[#D32F2F]' : 'border-[#CBD2D9]'}
				{locked ? 'pointer-events-none opacity-60' : 'cursor-pointer'}"
			onclick={() => input?.click()}
		>
			<span class="grid size-9 place-items-center rounded-full bg-[#FDECEC] text-[#D32F2F]">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8">
					<path d="M12 16V4" /><path d="m7 9 5-5 5 5" /><path d="M4 17v2a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-2" />
				</svg>
			</span>
			<span class="text-[13px] font-semibold text-[#1A2530]">
				{uploading ? t('ky.file.uploadingCap') : t('ky.file.upload')}
			</span>
			<span class="text-[11px] text-[#7A8894]">{hint}</span>
		</button>
		<input
			type="file"
			class="hidden"
			bind:this={input}
			{accept}
			onchange={(e) => {
				const f = (e.currentTarget as HTMLInputElement).files?.[0];
				pick(f);
				if (input) input.value = '';
			}}
		/>
	{/if}
	{#if zoom && thumb}
		<ImageLightbox src={thumb} alt={label} onclose={() => (zoom = false)} />
	{/if}
	{#if finalErr}
		<p class="mt-1 text-[12px] font-medium text-[#D32F2F]">{finalErr}</p>
	{/if}
</div>
