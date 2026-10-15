<script lang="ts">
	import { getI18n } from '../../i18n/i18n.svelte';
	const { t } = getI18n();
	import Combobox from '../Combobox.svelte';
	// Dropdown gaya DOKU: 40px, radius 6px, latar putih; border merah saat error.
	let {
		id,
		label,
		value = $bindable(''),
		options = [],
		placeholder = '',
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
	<Combobox
		{id}
		name={id}
		{options}
		bind:value
		{disabled}
		placeholder={placeholder || t('ky.sel.pick')}
		inputClass="h-10 w-full rounded-md border bg-white px-3 text-[14px] text-[#1A2530] outline-none transition focus:border-[#D32F2F] focus:ring-2 focus:ring-[#D32F2F]/15 disabled:opacity-60 {error ? 'border-[#D32F2F]' : 'border-[#CBD2D9]'}"
	/>
	{#if error}
		<p class="mt-1 text-[12px] font-medium text-[#D32F2F]">{error}</p>
	{:else if hint}
		<p class="mt-1 text-[12px] text-[#7A8894]">{hint}</p>
	{/if}
</div>
