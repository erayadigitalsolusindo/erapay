<script lang="ts">
	// Ombak berlapis di tepi atas section: gelombang gabungan beberapa sinus (bukan satu pola berulang),
	// tiap lapis bergeser dengan kecepatan berbeda, puncaknya diberi buih. Menimpa section di atasnya.
	// Lebar path 2880 = dua kali 1440, semua frekuensi bulat per 1440 sehingga mulus saat berulang.
	const lapis = [
		{ warna: '#2b8fb0', op: 0.45, dur: 26, arah: 1, y: 34, amp: [[2, 14, 0], [5, 6, 1.3]], buih: false },
		{ warna: '#1a729a', op: 0.7, dur: 19, arah: -1, y: 52, amp: [[3, 12, 2], [7, 5, 0.4]], buih: true },
		{ warna: '#0b4f6c', op: 1, dur: 14, arah: 1, y: 70, amp: [[2, 10, 4], [4, 8, 2.2], [9, 3, 1]], buih: true }
	];

	function path(y: number, amp: number[][]) {
		const pts: string[] = [];
		for (let x = 0; x <= 2880; x += 24) {
			let v = y;
			for (const [k, a, f] of amp) v += a * Math.sin(((2 * Math.PI * k * x) / 1440) + f);
			pts.push(`${x} ${v.toFixed(1)}`);
		}
		return 'M' + pts.join('L');
	}
	const bentuk = lapis.map((l) => {
		const garis = path(l.y, l.amp);
		return { ...l, garis, isi: `${garis}L2880 140L0 140Z` };
	});
</script>

<div class="pointer-events-none absolute inset-x-0 -top-[4.5rem] z-10 h-[9rem] overflow-hidden sm:-top-24" aria-hidden="true">
	{#each bentuk as l}
		<svg class="ombak absolute top-0 left-0 h-full w-[200%]" style="animation-duration:{l.dur}s;animation-direction:{l.arah > 0 ? 'normal' : 'reverse'};opacity:{l.op}" viewBox="0 0 2880 140" preserveAspectRatio="none">
			<path d={l.isi} fill={l.warna} />
			{#if l.buih}<path d={l.garis} fill="none" stroke="#fff" stroke-opacity=".55" stroke-width="3" stroke-linejoin="round" />{/if}
		</svg>
	{/each}
</div>

<style>
	.ombak { animation: geser linear infinite; will-change: transform; }
	@keyframes geser { to { transform: translateX(-50%); } }
	@media (prefers-reduced-motion: reduce) { .ombak { animation: none; } }
</style>
