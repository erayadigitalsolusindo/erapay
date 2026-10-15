<script lang="ts">
	import { getI18n } from '../../i18n/i18n.svelte';
	const { t } = getI18n();
	// Input teks gaya DOKU: 40px, radius 6px; mode baca pakai latar #E5E8ED.
	// `masked` menampilkan nilai ter-segel (NIK/NPWP) sebagai read-only.
	let {
		id,
		label,
		value = $bindable(''),
		type = 'text',
		placeholder = '',
		hint = '',
		error = '',
		max = 0, // > 0: tampilkan penghitung karakter
		readonly = false,
		masked = false,
		inputmode = undefined,
		autocomplete = undefined
	}: {
		id: string;
		label: string;
		value?: string;
		type?: string;
		placeholder?: string;
		hint?: string;
		error?: string;
		max?: number;
		readonly?: boolean;
		masked?: boolean;
		inputmode?: 'numeric' | 'tel' | 'text' | 'email' | 'decimal';
		autocomplete?: AutoFill;
	} = $props();
	const over = $derived(max > 0 && value.length > max);
</script>

<div class="w-full">
	<label for={id} class="mb-1.5 block text-[13px] font-semibold text-[#1A2530]">{label}</label>
	{#if masked}
		<div
			id={id}
			class="flex h-10 w-full items-center rounded-md border border-[#CBD2D9] bg-[#E5E8ED] px-3 text-[14px] tracking-widest text-[#42505C]"
		>
			{value || '—'}
			<span class="ml-auto text-[11px] font-medium tracking-normal text-[#7A8894]">{t('ky.tf.closed')}</span>
		</div>
	{:else}
		<input
			{id}
			{type}
			{placeholder}
			{inputmode}
			{autocomplete}
			bind:value
			readonly={readonly || undefined}
			aria-invalid={!!error}
			aria-describedby={error ? `${id}-err` : hint ? `${id}-hint` : undefined}
			class="h-10 w-full rounded-md border px-3 text-[14px] text-[#1A2530] outline-none transition
				placeholder:text-[#9AA6B2] focus:border-[#D32F2F] focus:ring-2 focus:ring-[#D32F2F]/15 disabled:cursor-not-allowed
				{readonly ? 'bg-[#E5E8ED] border-[#CBD2D9] text-[#42505C]' : 'bg-white'}
				{error ? 'border-[#D32F2F]' : 'border-[#CBD2D9]'}"
		/>
	{/if}
	<div class="mt-1 flex items-start justify-between gap-3">
		{#if error}
			<p id="{id}-err" class="text-[12px] font-medium text-[#D32F2F]">{error}</p>
		{:else if hint}
			<p id="{id}-hint" class="text-[12px] text-[#7A8894]">{hint}</p>
		{:else}
			<span></span>
		{/if}
		{#if max > 0}
			<p class="ml-auto shrink-0 text-[11px] tabular-nums {over ? 'text-[#D32F2F]' : 'text-[#9AA6B2]'}">
				{value.length}/{max}
			</p>
		{/if}
	</div>
</div>
