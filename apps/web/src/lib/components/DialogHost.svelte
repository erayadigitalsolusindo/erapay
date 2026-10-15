<script lang="ts">
	// Host dialog konfirmasi + toast beranimasi (pengganti confirm/alert bawaan browser).
	// Setiap varian punya ikon dan gerak sendiri: info (berayun), success (centang digambar + percikan),
	// warning (segitiga bergetar), danger (tong sampah, tutup terangkat).
	import { ui, closeConfirm, dismissToast, DEFAULT_TITLE, type Variant, type ToastVariant } from '../ui/dialog.svelte';

	const c = $derived(ui.confirm);
	const variant = $derived<Variant>(c?.variant ?? 'warning');
	const okLabel = $derived(c?.confirmText ?? (variant === 'danger' ? 'Ya, lanjutkan' : 'Ya, lanjut'));

	const TONE: Record<Variant, { ring: string; btn: string; soft: string; text: string }> = {
		info: { ring: '#2563eb', btn: 'bg-blue-600 hover:bg-blue-700', soft: 'bg-blue-50', text: 'text-blue-600' },
		success: { ring: '#059669', btn: 'bg-emerald-600 hover:bg-emerald-700', soft: 'bg-emerald-50', text: 'text-emerald-600' },
		warning: { ring: '#d97706', btn: 'bg-amber-500 hover:bg-amber-600', soft: 'bg-amber-50', text: 'text-amber-600' },
		danger: { ring: '#dc2626', btn: 'bg-red-600 hover:bg-red-700', soft: 'bg-red-50', text: 'text-red-600' }
	};

	// Gaya toast: bar berwarna dengan garis aksen kiri, ikon bulat terisi, dan tombol tutup bulat.
	const TOAST: Record<ToastVariant, { bg: string; fg: string; edge: string; glyph: 'check' | 'x' | 'i' | 'bang' }> = {
		default: { bg: '#ececec', fg: '#8f8f8f', edge: '#bdbdbd', glyph: 'check' },
		success: { bg: '#bff3d6', fg: '#1a9f55', edge: '#2ecc71', glyph: 'check' },
		info: { bg: '#d4ecff', fg: '#2fa8f5', edge: '#7fc8ff', glyph: 'check' },
		danger: { bg: '#ffdede', fg: '#f0384a', edge: '#ff4b5c', glyph: 'x' },
		warning: { bg: '#ffd996', fg: '#cf8200', edge: '#ff9f0a', glyph: 'bang' },
		dark: { bg: '#4b4b4b', fg: '#dcdcdc', edge: '#2c2c2c', glyph: 'bang' }
	};

	let cancelBtn = $state<HTMLButtonElement>();
	let okBtn = $state<HTMLButtonElement>();
	$effect(() => {
		// Aksi berisiko: fokus awal ke "Batal" supaya Enter tidak langsung menghapus.
		if (c) (variant === 'danger' ? cancelBtn : okBtn)?.focus();
	});

	function onKey(e: KeyboardEvent) {
		if (!c) return;
		if (e.key === 'Escape') closeConfirm(false);
		else if (e.key === 'Tab') {
			// jebak fokus di dalam dialog
			const els = [cancelBtn, okBtn].filter(Boolean) as HTMLElement[];
			const i = els.indexOf(document.activeElement as HTMLElement);
			e.preventDefault();
			els[(i + (e.shiftKey ? -1 : 1) + els.length) % els.length]?.focus();
		}
	}
</script>

<svelte:window onkeydown={onKey} />

{#snippet icon(v: Variant, size: number)}
	<svg viewBox="0 0 64 64" width={size} height={size} fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		{#if v === 'success'}
			<circle class="dh-draw" cx="32" cy="32" r="26" pathLength="1" />
			<path class="dh-draw dh-d2" d="m20 33 8 8 16-17" pathLength="1" />
		{:else if v === 'danger'}
			<g class="dh-bin">
				<path d="M18 22h28l-2.6 27a4 4 0 0 1-4 3.6H24.6a4 4 0 0 1-4-3.6L18 22Z" />
				<path d="M27 29v16M37 29v16" />
				<g class="dh-lid"><path d="M14 22h36" /><path d="M26 22v-4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v4" /></g>
			</g>
		{:else if v === 'warning'}
			<g class="dh-shake">
				<path d="M32 9 5.5 54h53L32 9Z" />
				<path d="M32 26v13" /><circle cx="32" cy="46" r="0.8" fill="currentColor" />
			</g>
		{:else}
			<g class="dh-bob">
				<circle cx="32" cy="32" r="26" />
				<path d="M32 29v14" /><circle cx="32" cy="21.5" r="0.8" fill="currentColor" />
			</g>
		{/if}
	</svg>
{/snippet}

{#if c}
	<div class="dh-backdrop fixed inset-0 z-[100] grid place-items-center overflow-y-auto bg-black/55 p-4 backdrop-blur-[2px]" role="presentation" onmousedown={(e) => e.target === e.currentTarget && closeConfirm(false)}>
		<div class="dh-card relative w-full max-w-sm overflow-hidden rounded-2xl bg-white p-6 pt-9 text-center shadow-2xl" role="alertdialog" aria-modal="true" aria-labelledby="dh-title" aria-describedby="dh-msg" style:--tone={TONE[variant].ring}>
			<span class="dh-band absolute inset-x-0 top-0 h-1.5" style:background={TONE[variant].ring}></span>
			<div class="dh-icon relative mx-auto grid size-20 place-items-center rounded-full {TONE[variant].soft} {TONE[variant].text}">
				<span class="dh-pulse absolute inset-0 rounded-full"></span>
				{@render icon(variant, 44)}
				{#if variant === 'success'}
					{#each [0, 1, 2, 3, 4, 5] as i (i)}<span class="dh-spark" style:--a="{i * 60}deg"></span>{/each}
				{/if}
			</div>
			<h3 id="dh-title" class="mt-4 text-base font-extrabold text-neutral-900">{c.title ?? DEFAULT_TITLE[variant]}</h3>
			<p id="dh-msg" class="mt-1.5 text-sm leading-relaxed whitespace-pre-line text-neutral-600">{c.message}</p>
			<div class="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-center">
				<button bind:this={cancelBtn} type="button" class="rounded-xl border border-neutral-200 bg-white px-5 py-2.5 text-sm font-bold text-neutral-700 transition hover:bg-neutral-100 focus-visible:ring-4 focus-visible:ring-neutral-300 sm:min-w-28" onclick={() => closeConfirm(false)}>{c.cancelText ?? 'Batal'}</button>
				<button bind:this={okBtn} type="button" class="rounded-xl px-5 py-2.5 text-sm font-bold text-white transition focus-visible:ring-4 focus-visible:ring-neutral-300 sm:min-w-28 {TONE[variant].btn}" onclick={() => closeConfirm(true)}>{okLabel}</button>
			</div>
		</div>
	</div>
{/if}

{#snippet glyph(g: 'check' | 'x' | 'i' | 'bang')}
	<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		{#if g === 'check'}<path d="m7 12.5 3.2 3.2L17 9" />
		{:else if g === 'x'}<path d="m8 8 8 8M16 8l-8 8" />
		{:else}<path d="M12 7v6" /><circle cx="12" cy="17" r="0.6" fill="#fff" />{/if}
	</svg>
{/snippet}

<div class="pointer-events-none fixed top-4 right-4 z-[110] flex w-[min(92vw,24rem)] flex-col gap-3" aria-live="polite">
	{#each ui.toasts as t (t.id)}
		{@const k = TOAST[t.variant]}
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div
			class="dh-toast pointer-events-auto relative flex cursor-pointer items-center gap-3 overflow-hidden rounded-md py-3 pr-4 pl-4 shadow-md"
			role="status"
			title="Klik untuk menutup"
			style="background:{k.bg};color:{k.fg};border-left:5px solid {k.edge}"
			onclick={() => dismissToast(t.id)}
		>
			<span class="grid size-7 shrink-0 place-items-center rounded-full" style="background:{k.fg}">{@render glyph(k.glyph)}</span>
			<div class="min-w-0">
				<p class="text-[13px] leading-tight font-extrabold">{t.title}</p>
				<p class="mt-0.5 text-xs leading-snug font-semibold break-words opacity-90">{t.message}</p>
			</div>
			<span class="dh-bar absolute inset-x-0 bottom-0 h-0.5" style="background:{k.fg};opacity:.35"></span>
		</div>
	{/each}
</div>

<style>
	.dh-backdrop { animation: dh-fade 0.18s ease-out; }
	.dh-card { animation: dh-pop 0.38s cubic-bezier(0.2, 1.2, 0.3, 1); }
	.dh-toast { animation: dh-slide 0.4s cubic-bezier(0.2, 1, 0.3, 1); }
	.dh-bar { transform-origin: left; animation: dh-shrink 4.2s linear forwards; }
	.dh-icon { animation: dh-icon-in 0.5s cubic-bezier(0.2, 1.4, 0.4, 1) 0.08s both; }
	.dh-pulse { background: var(--tone); opacity: 0; animation: dh-ping 1.8s ease-out 0.4s infinite; }

	.dh-draw { stroke-dasharray: 1; stroke-dashoffset: 1; animation: dh-draw 0.55s ease-out 0.25s forwards; }
	.dh-d2 { animation-delay: 0.62s; animation-duration: 0.35s; }
	.dh-spark {
		position: absolute; left: 50%; top: 50%; width: 6px; height: 6px; margin: -3px; border-radius: 9999px;
		background: var(--tone); opacity: 0; transform: rotate(var(--a)) translateY(0);
		animation: dh-burst 0.7s ease-out 0.7s forwards;
	}
	.dh-lid { transform-box: fill-box; transform-origin: 0% 100%; animation: dh-lid 1.6s ease-in-out 0.5s infinite; }
	.dh-bin { transform-origin: 50% 100%; animation: dh-wobble 1.6s ease-in-out 0.5s infinite; }
	.dh-shake { transform-origin: 50% 90%; animation: dh-shake 1.8s ease-in-out 0.5s infinite; }
	.dh-bob { transform-origin: 50% 100%; animation: dh-bob 1.8s ease-in-out 0.5s infinite; }

	@keyframes dh-fade { from { opacity: 0; } to { opacity: 1; } }
	@keyframes dh-pop { from { opacity: 0; transform: translateY(14px) scale(0.9); } to { opacity: 1; transform: none; } }
	@keyframes dh-slide { from { opacity: 0; transform: translateX(40px) scale(0.96); } to { opacity: 1; transform: none; } }
	@keyframes dh-shrink { from { transform: scaleX(1); } to { transform: scaleX(0); } }
	@keyframes dh-icon-in { from { transform: scale(0.3) rotate(-25deg); opacity: 0; } to { transform: none; opacity: 1; } }
	@keyframes dh-ping { 0% { transform: scale(1); opacity: 0.35; } 100% { transform: scale(1.55); opacity: 0; } }
	@keyframes dh-draw { to { stroke-dashoffset: 0; } }
	@keyframes dh-burst { 0% { opacity: 1; transform: rotate(var(--a)) translateY(-34px) scale(1); } 100% { opacity: 0; transform: rotate(var(--a)) translateY(-54px) scale(0.2); } }
	@keyframes dh-lid { 0%, 55%, 100% { transform: rotate(0); } 15%, 40% { transform: rotate(-24deg) translateY(-2px); } }
	@keyframes dh-wobble { 0%, 60%, 100% { transform: rotate(0); } 20% { transform: rotate(-3deg); } 30% { transform: rotate(3deg); } 40% { transform: rotate(-2deg); } }
	@keyframes dh-shake { 0%, 50%, 100% { transform: rotate(0); } 10% { transform: rotate(-7deg); } 20% { transform: rotate(7deg); } 30% { transform: rotate(-5deg); } 40% { transform: rotate(4deg); } }
	@keyframes dh-bob { 0%, 55%, 100% { transform: translateY(0); } 20% { transform: translateY(-5px); } 35% { transform: translateY(0); } 45% { transform: translateY(-2px); } }

	@media (prefers-reduced-motion: reduce) {
		.dh-backdrop, .dh-card, .dh-toast, .dh-icon, .dh-pulse, .dh-lid, .dh-bin, .dh-shake, .dh-bob, .dh-spark { animation: none; }
		.dh-bar { animation-duration: 4.2s; }
		.dh-draw { animation: none; stroke-dashoffset: 0; }
	}
</style>
