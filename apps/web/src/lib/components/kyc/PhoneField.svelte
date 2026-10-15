<script lang="ts">
	// Telepon Indonesia: awalan tetap +62, nilai disimpan tanpa 62/0 di depan (server yang menormalkan).
	let {
		id,
		label,
		value = $bindable(''),
		placeholder = '812 3456 7890',
		hint = '',
		error = ''
	}: {
		id: string;
		label: string;
		value?: string;
		placeholder?: string;
		hint?: string;
		error?: string;
	} = $props();
</script>

<div class="w-full">
	<label for={id} class="mb-1.5 block text-[13px] font-semibold text-[#1A2530]">{label}</label>
	<div
		class="flex h-10 overflow-hidden rounded-md border transition focus-within:border-[#D32F2F] focus-within:ring-2 focus-within:ring-[#D32F2F]/15
			{error ? 'border-[#D32F2F]' : 'border-[#CBD2D9]'}"
	>
		<span class="grid place-items-center border-r border-[#CBD2D9] bg-[#E5E8ED] px-3 text-[14px] font-semibold text-[#42505C]">
			+62
		</span>
		<input
			{id}
			{placeholder}
			inputmode="tel"
			autocomplete="tel-national"
			bind:value
			aria-invalid={!!error}
			class="h-full w-full bg-white px-3 text-[14px] text-[#1A2530] outline-none placeholder:text-[#9AA6B2]"
		/>
	</div>
	{#if error}
		<p class="mt-1 text-[12px] font-medium text-[#D32F2F]">{error}</p>
	{:else if hint}
		<p class="mt-1 text-[12px] text-[#7A8894]">{hint}</p>
	{/if}
</div>
