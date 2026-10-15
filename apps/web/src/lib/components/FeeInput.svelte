<script lang="ts">
	// Isian tarif: nilai + pilihan satuan per komponen (persen dari transaksi atau nominal rupiah).
	// Field terkirim sebagai `${name}_value` dan `${name}_unit` ("percent" | "rupiah"); server yang mengonversi.
	let { name, label, hint = '' }: { name: string; label: string; hint?: string } = $props();
	let unit = $state<'percent' | 'rupiah'>('percent');
</script>

<div class="min-w-0 flex-1 basis-56">
	<label class="mb-1 block text-[11px] font-bold" for={`${name}-value`}>{label}</label>
	<div class="flex overflow-hidden rounded-xl border border-ink/15 bg-surface focus-within:ring-4 focus-within:ring-emas/60">
		<input
			id={`${name}-value`}
			name={`${name}_value`}
			class="min-w-0 flex-1 bg-transparent px-3 py-2 text-xs outline-none"
			inputmode="decimal"
			autocomplete="off"
			required
			placeholder={unit === 'percent' ? 'mis. 0,7' : 'mis. 500'}
			aria-describedby={hint ? `${name}-hint` : undefined}
		/>
		<div class="flex shrink-0 border-l border-ink/15 text-[11px] font-bold" role="radiogroup" aria-label={`Satuan ${label}`}>
			{#each [['percent', '%'], ['rupiah', 'Rp']] as [v, text] (v)}
				<label class="cursor-pointer select-none px-3 py-2 transition {unit === v ? 'bg-merah text-white' : 'hover:bg-ink/5'}">
					<input type="radio" class="sr-only" name={`${name}_unit`} value={v} checked={unit === v} onchange={() => (unit = v as 'percent' | 'rupiah')} />
					{text}
				</label>
			{/each}
		</div>
	</div>
	{#if hint}<p id={`${name}-hint`} class="muted m-0 mt-1 text-[10px]">{hint}</p>{/if}
</div>
