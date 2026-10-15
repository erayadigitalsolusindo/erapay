<script lang="ts">
	// Dropdown gaya DOKU: 40px, radius 6px, latar putih; border merah saat error.
	let {
		id,
		label,
		value = $bindable(''),
		options = [],
		placeholder = 'Pilih',
		hint = '',
		error = '',
		disabled = false
	}: {
		id: string;
		label: string;
		value?: string;
		options: { value: string; label: string }[];
		placeholder?: string;
		hint?: string;
		error?: string;
		disabled?: boolean;
	} = $props();
</script>

<div class="w-full">
	<label for={id} class="mb-1.5 block text-[13px] font-semibold text-[#1A2530]">{label}</label>
	<select
		{id}
		bind:value
		{disabled}
		aria-invalid={!!error}
		class="h-10 w-full appearance-none rounded-md border bg-white bg-[url('data:image/svg+xml;utf8,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%2212%22 height=%2212%22 viewBox=%220 0 24 24%22 fill=%22none%22 stroke=%22%237A8894%22 stroke-width=%222%22><path d=%22m6 9 6 6 6-6%22/></svg>')] bg-[position:right_10px_center] bg-no-repeat px-3 pr-8 text-[14px] text-[#1A2530] outline-none transition focus:border-[#D32F2F] focus:ring-2 focus:ring-[#D32F2F]/15
			{error ? 'border-[#D32F2F]' : 'border-[#CBD2D9]'}"
	>
		<option value="" disabled>{placeholder}</option>
		{#each options as o (o.value)}
			<option value={o.value}>{o.label}</option>
		{/each}
	</select>
	{#if error}
		<p class="mt-1 text-[12px] font-medium text-[#D32F2F]">{error}</p>
	{:else if hint}
		<p class="mt-1 text-[12px] text-[#7A8894]">{hint}</p>
	{/if}
</div>
