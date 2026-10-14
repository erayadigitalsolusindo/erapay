<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		id: string;
		name: string;
		label: string;
		icon: 'mail' | 'lock' | 'user' | 'phone' | 'store' | 'card' | 'pin';
		type?: string;
		value?: string;
		placeholder?: string;
		autocomplete?: AutoFill;
		error?: string;
		required?: boolean;
		compact?: boolean;
		prefix?: string;
		inputmode?: 'numeric' | 'tel' | 'text' | 'email' | 'decimal';
		trailing?: Snippet;
		below?: Snippet;
		oninput?: (e: Event) => void;
	};

	let {
		id, name, label, icon, type = 'text', value = $bindable(''), placeholder, autocomplete,
		error, required = true, compact = false, prefix, inputmode, trailing, below, oninput
	}: Props = $props();

	const PATHS: Record<string, string> = {
		mail: '<rect x="3" y="5" width="18" height="14" rx="3"/><path d="m4 7 8 6 8-6"/>',
		lock: '<rect x="4" y="10" width="16" height="10" rx="3"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>',
		user: '<circle cx="12" cy="8" r="4"/><path d="M4 21c0-4 4-6 8-6s8 2 8 6"/>',
		phone: '<rect x="7" y="2" width="10" height="20" rx="3"/><path d="M11 18h2"/>',
		store: '<path d="M4 9l1.5-5h13L20 9"/><path d="M4 9v11h16V9"/><path d="M4 9a2.7 2.7 0 0 0 5.3 0 2.7 2.7 0 0 0 5.4 0A2.7 2.7 0 0 0 20 9"/>',
		card: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 10h18"/><path d="M7 15h4"/>',
		pin: '<path d="M12 21s7-6.4 7-11a7 7 0 1 0-14 0c0 4.6 7 11 7 11Z"/><circle cx="12" cy="10" r="2.5"/>'
	};
</script>

<div>
	<label for={id} class="block text-sm font-semibold text-neutral-700 {compact ? 'mb-1' : 'mb-1.5'}">{label}</label>
	<div class="group relative">
		<span class="pointer-events-none absolute inset-y-0 left-4 grid place-items-center text-neutral-400 transition group-focus-within:text-merah">
			<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">{@html PATHS[icon]}</svg>
		</span>
		{#if prefix}
			<span class="pointer-events-none absolute inset-y-0 left-12 grid place-items-center text-sm font-semibold text-neutral-500">{prefix}</span>
		{/if}
		<input
			{id} {name} {type} {placeholder} {autocomplete} {required} {oninput}
			inputmode={inputmode ?? undefined}
			bind:value
			aria-invalid={error ? 'true' : undefined}
			aria-describedby={error ? `${id}-err` : undefined}
			class="w-full rounded-xl border bg-white pr-12 text-sm shadow-sm transition outline-none placeholder:text-neutral-400 focus:border-merah focus:ring-4 focus:ring-merah/15
				{compact ? 'py-2.5' : 'py-3'} {prefix ? 'pl-[5.25rem]' : 'pl-12'} {error ? 'border-merah' : 'border-neutral-300'}"
		/>
		{@render trailing?.()}
	</div>
	{@render below?.()}
	{#if error}
		<p id="{id}-err" class="mt-1.5 text-xs font-medium text-merah">{error}</p>
	{/if}
</div>
