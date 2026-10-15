<script lang="ts">
	// Galeri bukti bisnis: thumbnail 72px, tombol hapus, tombol tambah (gambar ≤ 5 MB).
	import type { Evidence } from '../../server/kyc';
	import { getI18n } from '../../i18n/i18n.svelte';
	import ImageLightbox from './ImageLightbox.svelte';
	const { t } = getI18n();


	let {
		evidences = [],
		busy = false,
		locked = false,
		error = '',
		onadd,
		ondelete
	}: {
		evidences: Evidence[];
		busy?: boolean;
		locked?: boolean;
		error?: string;
		onadd: (f: File) => void | Promise<void>;
		ondelete: (id: string) => void;
	} = $props();

	let input: HTMLInputElement | undefined;
	// Pratinjau berkas yang baru dipilih (diunggah satu per satu) sampai muncul dari server.
	let pending = $state<{ key: number; url: string; name: string }[]>([]);
	let seq = 0;
	let zoomed = $state<Evidence | null>(null);

	async function addMany(files: File[]) {
		const batch = files.map((f) => ({ key: seq++, url: URL.createObjectURL(f), name: f.name, f }));
		pending = [...pending, ...batch.map(({ f: _f, ...p }) => p)];
		for (const b of batch) {
			try {
				await onadd(b.f);
			} finally {
				pending = pending.filter((p) => p.key !== b.key);
				URL.revokeObjectURL(b.url);
			}
		}
	}
</script>

<div class="w-full">
	<label class="mb-1.5 block text-[13px] font-semibold text-[#1A2530]">{t('ky.gal.title')}</label>
	<p class="mb-2 text-[12px] text-[#7A8894]">{t('ky.gal.note')}</p>
	<div class="flex flex-wrap gap-2">
		{#each evidences as ev (ev.id)}
			<div class="group relative size-[72px] overflow-hidden rounded-md border border-[#CBD2D9] bg-white">
				<button type="button" class="size-full cursor-zoom-in" aria-label={t('ky.gal.zoom', { name: ev.name })} onclick={() => (zoomed = ev)}>
					<img src={ev.url} alt={ev.name} class="size-full object-cover" />
				</button>
				{#if !locked}
					<button
						type="button"
						aria-label={t('ky.gal.remove')}
						class="absolute right-1 top-1 hidden size-5 place-items-center rounded-full bg-[#D32F2F] text-white group-hover:grid"
						onclick={() => ondelete(ev.id)}
					>
						<svg viewBox="0 0 24 24" class="size-3" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M18 6 6 18M6 6l12 12" /></svg>
					</button>
				{/if}
			</div>
		{/each}
		{#each pending as p (p.key)}
			<div class="relative size-[72px] overflow-hidden rounded-md border border-[#CBD2D9] bg-white">
				<img src={p.url} alt={p.name} class="size-full object-cover opacity-60" />
				<span class="absolute inset-0 grid place-items-center bg-white/40 text-[10px] font-semibold text-[#42505C]">{t('ky.uploading')}</span>
			</div>
		{/each}
		{#if !locked}
			<button
				type="button"
				class="grid size-[72px] place-items-center rounded-md border-2 border-dashed border-[#CBD2D9] text-[#7A8894] transition hover:border-[#D32F2F]/60 hover:text-[#D32F2F]"
				aria-label={t('ky.gal.add')}
				onclick={() => input?.click()}
			>
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14" /></svg>
			</button>
		{/if}
		{#each Array.from({ length: Math.max(0, 3 - evidences.length - pending.length - (locked ? 0 : 1)) }) as _, i (i)}
			<!-- placeholder kotak kosong agar galeri terasa seperti galeri -->
			<div class="size-[72px] rounded-md border border-dashed border-[#E5E8ED]"></div>
		{/each}
	</div>
	<input
		type="file"
		accept=".jpg,.jpeg,.png,image/jpeg,image/png"
		multiple
		bind:this={input}
		class="hidden"
		onchange={(e) => {
			const inp = e.currentTarget as HTMLInputElement;
			const fs = Array.from(inp.files ?? []);
			inp.value = '';
			if (fs.length) void addMany(fs);
		}}
	/>
	{#if zoomed}
		<ImageLightbox src={zoomed.url} alt={zoomed.name} onclose={() => (zoomed = null)} />
	{/if}
	{#if busy}
		<p class="mt-1 text-[12px] text-[#7A8894]">{t('ky.uploading')}</p>
	{/if}
	{#if error}
		<p class="mt-1 text-[12px] font-medium text-[#D32F2F]">{error}</p>
	{/if}
</div>
