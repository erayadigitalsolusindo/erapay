<script lang="ts">
	// Daftar tautan jualan (maks 5): tambah/hapus, urutan sesuai isian.
	let {
		links = [],
		locked = false,
		error = '',
		max = 5,
		onchange
	}: {
		links: string[];
		locked?: boolean;
		error?: string;
		onchange: (links: string[]) => void;
		max?: number;
	} = $props();

	function add() {
		if (locked || links.length >= max) return;
		onchange([...links, '']);
	}
	function remove(i: number) {
		onchange(links.filter((_, j) => j !== i));
	}
</script>

<div class="w-full">
	<div class="mb-1.5 flex items-baseline justify-between">
		<label class="text-[13px] font-semibold text-[#1A2530]">Tautan tempat berjualan</label>
		<span class="text-[11px] text-[#9AA6B2]">{links.length}/{max}</span>
	</div>
	<div class="space-y-2">
		{#each links as link, i (i)}
			<div class="flex gap-2">
				<input
					value={link}
					oninput={(e) => {
						const v = (e.currentTarget as HTMLInputElement).value;
						onchange(links.map((l, j) => (j === i ? v : l)));
					}}
					disabled={locked}
					placeholder="https://tokoanda.com"
					inputmode="url"
					class="h-10 w-full rounded-md border bg-white px-3 text-[14px] text-[#1A2530] outline-none transition
						placeholder:text-[#9AA6B2] focus:border-[#D32F2F] focus:ring-2 focus:ring-[#D32F2F]/15 border-[#CBD2D9]
						{locked ? 'bg-[#E5E8ED] text-[#42505C]' : ''}"
				/>
				{#if !locked}
					<button
						type="button"
						aria-label="Hapus tautan"
						class="grid size-10 shrink-0 place-items-center rounded-md border border-[#CBD2D9] bg-white text-[#7A8894] transition hover:border-[#D32F2F] hover:text-[#D32F2F]"
						onclick={() => remove(i)}
					>
						<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12" /></svg>
					</button>
				{/if}
			</div>
		{/each}
	</div>
	{#if !locked && links.length < max}
		<button
			type="button"
			class="mt-2 text-[13px] font-semibold text-[#D32F2F] hover:text-[#B02424]"
			onclick={add}
		>
			+ Tambah tautan
		</button>
	{/if}
	{#if error}
		<p class="mt-1 text-[12px] font-medium text-[#D32F2F]">{error}</p>
	{/if}
</div>
