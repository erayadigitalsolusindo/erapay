<!--
  Combobox yang bisa diketik/dicari (pengganti <select>). Nilai dikirim lewat <input type="hidden" name>
  sehingga tetap bekerja di <form> biasa maupun use:enhance. Keyboard: ↑/↓, Enter, Esc, Home/End.
-->
<script lang="ts">
	import { getI18n } from '../i18n/i18n.svelte';

	type Opt = { value: string; label: string; hint?: string };
	let {
		id,
		name,
		options,
		value = $bindable(''),
		placeholder = '',
		required = false,
		onchange
	}: {
		id: string;
		name: string;
		options: Opt[];
		value?: string;
		placeholder?: string;
		required?: boolean;
		onchange?: (v: string) => void;
	} = $props();

	const { t } = getI18n();
	let open = $state(false);
	let query = $state('');
	let active = $state(0);
	let root: HTMLDivElement;
	let input: HTMLInputElement;

	const selected = $derived(options.find((o) => o.value === value));
	const shown = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return q ? options.filter((o) => (o.label + ' ' + (o.hint ?? '') + ' ' + o.value).toLowerCase().includes(q)) : options;
	});

	function show() {
		open = true;
		query = '';
		active = Math.max(0, options.findIndex((o) => o.value === value));
	}
	function pick(o: Opt) {
		value = o.value;
		open = false;
		query = '';
		onchange?.(o.value);
	}
	function onKey(e: KeyboardEvent) {
		if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
			e.preventDefault();
			show();
			return;
		}
		if (!open) return;
		if (e.key === 'ArrowDown') (e.preventDefault(), (active = Math.min(active + 1, shown.length - 1)));
		else if (e.key === 'ArrowUp') (e.preventDefault(), (active = Math.max(active - 1, 0)));
		else if (e.key === 'Home') (e.preventDefault(), (active = 0));
		else if (e.key === 'End') (e.preventDefault(), (active = shown.length - 1));
		else if (e.key === 'Enter') {
			e.preventDefault(); // jangan submit form saat memilih
			if (shown[active]) pick(shown[active]);
		} else if (e.key === 'Escape') open = false;
		else if (e.key === 'Tab') open = false;
	}
	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => {
			if (!root.contains(e.target as Node)) open = false;
		};
		document.addEventListener('pointerdown', away);
		return () => document.removeEventListener('pointerdown', away);
	});
	$effect(() => {
		query; // saat mengetik, sorotan kembali ke hasil teratas
		active = 0;
	});
	$effect(() => {
		if (open) document.getElementById(`${id}-o${active}`)?.scrollIntoView({ block: 'nearest' });
	});
</script>

<div class="relative" bind:this={root}>
	<input type="hidden" {name} {value} />
	<div class="relative">
		<input
			{id}
			bind:this={input}
			class="inp pr-9"
			role="combobox"
			aria-expanded={open}
			aria-controls="{id}-list"
			aria-autocomplete="list"
			aria-activedescendant={open && shown[active] ? `${id}-o${active}` : undefined}
			autocomplete="off"
			placeholder={selected?.label || placeholder}
			required={required && !value}
			value={open ? query : (selected?.label ?? '')}
			oninput={(e) => {
				if (!open) open = true;
				query = e.currentTarget.value;
			}}
			onfocus={show}
			onclick={() => !open && show()}
			onkeydown={onKey}
		/>
		<button
			type="button"
			tabindex="-1"
			class="absolute inset-y-0 right-0 grid w-9 place-items-center text-ink/50"
			aria-label={t('cb.toggle')}
			onclick={() => (open ? (open = false) : (show(), input.focus()))}
		>
			<svg viewBox="0 0 24 24" class="size-4 transition {open ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
		</button>
	</div>
	{#if open}
		<ul
			id="{id}-list"
			role="listbox"
			class="absolute z-30 mt-1.5 max-h-64 w-full overflow-auto rounded-xl border border-ink/15 bg-surface p-1 text-sm shadow-lg"
		>
			{#each shown as o, i (o.value)}
				<!-- svelte-ignore a11y_click_events_have_key_events -->
				<li
					id="{id}-o{i}"
					role="option"
					aria-selected={o.value === value}
					class="flex cursor-pointer items-center justify-between gap-3 rounded-lg px-3 py-2 {i === active ? 'bg-merah/10' : ''} {o.value === value ? 'font-bold' : ''}"
					onpointerenter={() => (active = i)}
					onmousedown={(e) => e.preventDefault()}
					onclick={() => pick(o)}
				>
					<span class="truncate">{o.label}</span>
					{#if o.hint}<span class="muted shrink-0 text-[11px]">{o.hint}</span>{:else if o.value === value}<span class="text-merah" aria-hidden="true">✓</span>{/if}
				</li>
			{:else}
				<li class="muted px-3 py-2 text-xs">{t('cb.none')}</li>
			{/each}
		</ul>
	{/if}
</div>
