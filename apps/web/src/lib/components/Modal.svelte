<script lang="ts">
	// Dialog modal sederhana: klik latar / tombol tutup / Esc untuk menutup; fokus awal ke dialog.
	import type { Snippet } from 'svelte';
	import { tNow } from '../i18n/i18n.svelte';
	let { title, onclose, wide = false, children }: { title: string; onclose: () => void; wide?: boolean; children: Snippet } = $props();
	let dlg = $state<HTMLDivElement>();
	$effect(() => dlg?.focus());

	// Pindahkan ke root `.adm` (bukan <body>: style .card/.btn/.inp & tema hanya berlaku di dalam .adm).
	// Induk dengan transform/filter/backdrop-filter menjebak `position: fixed` sehingga modal
	// terpotong di dalam kartu alih-alih menutupi layar.
	function portal(node: HTMLElement) {
		(node.closest('.adm') ?? document.body).appendChild(node);
		return { destroy: () => node.remove() };
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div use:portal class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/50 p-4" role="presentation" onmousedown={(e) => e.target === e.currentTarget && onclose()}>
	<div
		bind:this={dlg}
		tabindex="-1"
		role="dialog"
		aria-modal="true"
		aria-label={title}
		class="card w-full min-w-0 {wide ? 'max-w-2xl' : 'max-w-lg'} break-words p-5 outline-none"
	>
		<div class="mb-4 flex items-start justify-between gap-3">
			<h3 class="text-sm font-bold">{title}</h3>
			<button type="button" aria-label={tNow('c.close')} class="grid size-7 place-items-center rounded-full hover:bg-ink/10" onclick={onclose}>
				<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M18 6 6 18M6 6l12 12" /></svg>
			</button>
		</div>
		{@render children()}
	</div>
</div>
