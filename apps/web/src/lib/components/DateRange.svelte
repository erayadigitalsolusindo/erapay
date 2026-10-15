<!--
  Pemilih rentang tanggal (date range) bergaya dasbor: tombol ringkas + popover kalender
  dengan preset. Nilai dikirim sebagai dua <input type="hidden"> (YYYY-MM-DD, WIB).
-->
<script lang="ts">
	import { getI18n } from '../i18n/i18n.svelte';

	let {
		from = $bindable(''),
		to = $bindable(''),
		nameFrom = 'from',
		nameTo = 'to',
		id = 'dr'
	}: { from?: string; to?: string; nameFrom?: string; nameTo?: string; id?: string } = $props();

	const i18n = getI18n();
	const { t } = i18n;

	const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	// "Hari ini" mengikuti WIB, bukan zona browser.
	const today = (() => {
		const p = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());
		return p;
	})();
	const parse = (s: string) => {
		const [y, m, d] = s.split('-').map(Number);
		return new Date(y, m - 1, d);
	};
	const addDays = (s: string, n: number) => {
		const d = parse(s);
		d.setDate(d.getDate() + n);
		return iso(d);
	};

	let open = $state(false);
	let a = $state(''); // draf awal
	let b = $state(''); // draf akhir
	let hover = $state('');
	let view = $state(new Date()); // bulan yang ditampilkan
	let root: HTMLDivElement;

	const label = (s: string) => parse(s).toLocaleDateString(i18n.locale, { day: 'numeric', month: 'short', year: 'numeric' });
	const summary = $derived(from && to ? (from === to ? label(from) : `${label(from)} – ${label(to)}`) : from ? `${label(from)} – …` : to ? `… – ${label(to)}` : '');

	function show() {
		a = from;
		b = to;
		hover = '';
		view = parse(from || today);
		view.setDate(1);
		open = true;
	}
	function pickDay(s: string) {
		if (!a || (a && b)) {
			a = s;
			b = '';
		} else if (s < a) {
			b = a;
			a = s;
		} else b = s;
	}
	function apply() {
		from = a;
		to = b || a;
		open = false;
	}
	function clear() {
		a = b = from = to = '';
		open = false;
	}
	function preset(days: number) {
		a = addDays(today, -days);
		b = today;
		view = parse(a);
		view.setDate(1);
	}
	function thisMonth(offset: number) {
		const t0 = parse(today);
		const s = new Date(t0.getFullYear(), t0.getMonth() + offset, 1);
		const e = new Date(t0.getFullYear(), t0.getMonth() + offset + 1, 0);
		a = iso(s);
		b = offset === 0 ? today : iso(e);
		view = s;
	}

	const monthName = $derived(view.toLocaleDateString(i18n.locale, { month: 'long', year: 'numeric' }));
	const weekdays = $derived(
		Array.from({ length: 7 }, (_, i) => new Date(2024, 0, 1 + i).toLocaleDateString(i18n.locale, { weekday: 'short' }))
	); // 1 Jan 2024 = Senin
	const cells = $derived.by(() => {
		const y = view.getFullYear();
		const m = view.getMonth();
		const lead = (new Date(y, m, 1).getDay() + 6) % 7; // minggu mulai Senin
		const n = new Date(y, m + 1, 0).getDate();
		return [...Array(lead).fill(null), ...Array.from({ length: n }, (_, i) => iso(new Date(y, m, i + 1)))] as (string | null)[];
	});
	const lo = $derived(a && !b && hover && hover < a ? hover : a);
	const hi = $derived(b || (a && hover && hover > a ? hover : a));
	const shift = (n: number) => (view = new Date(view.getFullYear(), view.getMonth() + n, 1));

	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => {
			if (!root.contains(e.target as Node)) open = false;
		};
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && (open = false);
		document.addEventListener('pointerdown', away);
		document.addEventListener('keydown', esc);
		return () => {
			document.removeEventListener('pointerdown', away);
			document.removeEventListener('keydown', esc);
		};
	});
</script>

<div class="relative" bind:this={root}>
	<input type="hidden" name={nameFrom} value={from} />
	<input type="hidden" name={nameTo} value={to} />
	<button
		type="button"
		{id}
		class="inp flex items-center justify-between gap-2 text-left"
		aria-haspopup="dialog"
		aria-expanded={open}
		onclick={() => (open ? (open = false) : show())}
	>
		<span class="flex min-w-0 items-center gap-2">
			<svg viewBox="0 0 24 24" class="size-4 shrink-0 text-ink/50" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M16 3v4M8 3v4M3 10h18" /></svg>
			<span class="truncate {summary ? '' : 'muted'}">{summary || t('dr.placeholder')}</span>
		</span>
		{#if summary}
			<span
				role="button"
				tabindex="0"
				aria-label={t('dr.clear')}
				class="grid size-5 shrink-0 place-items-center rounded-full text-ink/50 hover:bg-ink/10 hover:text-ink"
				onclick={(e) => (e.stopPropagation(), clear())}
				onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), e.stopPropagation(), clear())}
			>
				<svg viewBox="0 0 24 24" class="size-3" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg>
			</span>
		{/if}
	</button>

	{#if open}
		<div role="dialog" aria-label={t('dr.placeholder')} class="absolute left-0 z-30 mt-1.5 flex max-w-[calc(100vw-2rem)] flex-col rounded-2xl border border-ink/15 bg-surface p-3 shadow-xl sm:flex-row">
			<div class="flex shrink-0 gap-1 border-b border-ink/10 pb-3 sm:w-36 sm:flex-col sm:border-r sm:border-b-0 sm:pr-3 sm:pb-0 max-sm:flex-wrap">
				<button type="button" class="btn btn-light justify-start !px-3 !py-1.5" onclick={() => preset(0)}>{t('dr.today')}</button>
				<button type="button" class="btn btn-light justify-start !px-3 !py-1.5" onclick={() => preset(6)}>{t('dr.last7')}</button>
				<button type="button" class="btn btn-light justify-start !px-3 !py-1.5" onclick={() => preset(29)}>{t('dr.last30')}</button>
				<button type="button" class="btn btn-light justify-start !px-3 !py-1.5" onclick={() => thisMonth(0)}>{t('dr.thisMonth')}</button>
				<button type="button" class="btn btn-light justify-start !px-3 !py-1.5" onclick={() => thisMonth(-1)}>{t('dr.lastMonth')}</button>
			</div>
			<div class="pt-3 sm:pt-0 sm:pl-3">
				<div class="mb-2 flex items-center justify-between">
					<button type="button" class="grid size-8 place-items-center rounded-lg hover:bg-ink/5" aria-label={t('dr.prev')} onclick={() => shift(-1)}>
						<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="m15 6-6 6 6 6" /></svg>
					</button>
					<span class="text-xs font-bold capitalize">{monthName}</span>
					<button type="button" class="grid size-8 place-items-center rounded-lg hover:bg-ink/5" aria-label={t('dr.next')} onclick={() => shift(1)}>
						<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>
					</button>
				</div>
				<div class="grid grid-cols-7 text-center text-[10px] font-bold text-ink/50">
					{#each weekdays as w (w)}<span class="py-1">{w}</span>{/each}
				</div>
				<div class="grid grid-cols-7" role="grid" tabindex="-1" onpointerleave={() => (hover = '')}>
					{#each cells as c, i (i)}
						{#if c}
							{@const edge = c === lo || c === hi}
							{@const inside = lo && hi && c > lo && c < hi}
							<button
								type="button"
								class="relative h-9 text-xs {inside ? 'bg-merah/10' : ''} {c === lo && hi !== lo ? 'rounded-l-lg bg-merah/10' : ''} {c === hi && hi !== lo ? 'rounded-r-lg bg-merah/10' : ''}"
								aria-pressed={edge}
								aria-label={label(c)}
								onpointerenter={() => (hover = c)}
								onclick={() => pickDay(c)}
							>
								<span class="mx-auto grid size-8 place-items-center rounded-lg {edge ? 'bg-merah font-bold text-white' : 'hover:bg-ink/5'} {c === today && !edge ? 'ring-1 ring-merah/50' : ''}">{Number(c.slice(8))}</span>
							</button>
						{:else}
							<span></span>
						{/if}
					{/each}
				</div>
				<div class="mt-3 flex items-center justify-between gap-2 border-t border-ink/10 pt-3">
					<span class="muted text-[11px]">{a ? (b && b !== a ? `${label(a)} – ${label(b)}` : label(a)) : t('dr.pick')}</span>
					<span class="flex gap-2">
						<button type="button" class="btn btn-light !px-3 !py-1.5" onclick={clear}>{t('dr.reset')}</button>
						<button type="button" class="btn btn-red !px-3 !py-1.5" disabled={!a} onclick={apply}>{t('dr.apply')}</button>
					</span>
				</div>
			</div>
		</div>
	{/if}
</div>
