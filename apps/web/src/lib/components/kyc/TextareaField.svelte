<script lang="ts">
	// Area teks dengan penghitung karakter (default 500, sesuai batas alamat detail).
	let {
		id,
		label,
		value = $bindable(''),
		placeholder = '',
		max = 500,
		rows = 3,
		hint = '',
		error = '',
		readonly = false
	}: {
		id: string;
		label: string;
		value?: string;
		placeholder?: string;
		max?: number;
		rows?: number;
		hint?: string;
		error?: string;
		readonly?: boolean;
	} = $props();
	const over = $derived(value.length > 500);
</script>

<div class="w-full">
	<div class="mb-1.5 flex items-baseline justify-between">
		<label for={id} class="text-[13px] font-semibold text-[#1A2530]">{label}</label>
		<span class="text-[11px] tabular-nums {over ? 'text-[#D32F2F]' : 'text-[#9AA6B2]'}">
			{value.length}/{max}
		</span>
	</div>
	<textarea
		{id}
		{rows}
		bind:value
		readonly={readonly || undefined}
		aria-invalid={!!error}
		class="w-full resize-none rounded-md border px-3 py-2 text-[14px] leading-relaxed text-[#1A2530] outline-none transition
			placeholder:text-[#9AA6B2] focus:border-[#D32F2F] focus:ring-2 focus:ring-[#D32F2F]/15
			{readonly ? 'bg-[#E5E8ED] border-[#CBD2D9] text-[#42505C]' : 'bg-white'}
			{error || over ? 'border-[#D32F2F]' : 'border-[#CBD2D9]'}"
	>{value}</textarea>
	{#if error}
		<p class="mt-1 text-[12px] font-medium text-[#D32F2F]">{error}</p>
	{:else if hint}
		<p class="mt-1 text-[12px] text-[#7A8894]">{hint}</p>
	{/if}
</div>
