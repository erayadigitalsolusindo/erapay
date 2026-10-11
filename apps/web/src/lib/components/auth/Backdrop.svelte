<!-- Latar langit + awan, plus definisi SVG bersama (gradasi emas, bentuk awan) yang dipakai
     komponen lain lewat url(#emas) / #awan. Dekoratif, aria-hidden. -->
<svg width="0" height="0" class="absolute" aria-hidden="true" focusable="false">
	<defs>
		<linearGradient id="emas" x1="0" y1="0" x2="1" y2="1">
			<stop offset="0" stop-color="#fff3c4" />
			<stop offset=".5" stop-color="#e8b84a" />
			<stop offset="1" stop-color="#b8862a" />
		</linearGradient>
		<radialGradient id="gAwan" cx=".4" cy=".35" r=".75">
			<stop offset="0" stop-color="#ffffff" />
			<stop offset=".6" stop-color="#f3f8ff" />
			<stop offset="1" stop-color="#bcd3ee" />
		</radialGradient>
		<radialGradient id="gAwanBelakang" cx=".4" cy=".35" r=".8">
			<stop offset="0" stop-color="#e3edfa" />
			<stop offset="1" stop-color="#86a8d2" />
		</radialGradient>
		<g id="awan" fill="#fff">
			<circle cx="58" cy="62" r="26" /><circle cx="92" cy="46" r="34" /><circle cx="132" cy="58" r="30" />
			<circle cx="160" cy="68" r="20" /><rect x="40" y="62" width="135" height="26" rx="13" />
		</g>
	</defs>
</svg>

<div aria-hidden="true" class="fixed inset-0 -z-10 overflow-hidden">
	<div class="absolute inset-0 bg-[linear-gradient(to_bottom,var(--color-merah-gelap)_0%,var(--color-merah-tua)_28%,var(--color-merah)_62%,#ff7a7a_100%)]"></div>
	<div class="animate-denyut absolute bottom-[-14rem] left-1/2 size-[46rem] -translate-x-1/2 rounded-full bg-emas/35 blur-3xl"></div>
	{#each [[6, 280, 0.22, 110, -10], [18, 200, 0.3, 85, -50], [32, 340, 0.18, 130, -90], [48, 240, 0.28, 95, -30], [62, 380, 0.2, 140, -60], [76, 260, 0.35, 100, -80], [88, 320, 0.4, 120, -20]] as [top, width, opacity, dur, delay]}
		<svg
			viewBox="0 0 200 100"
			class="animate-awan absolute left-0 blur-[1px]"
			style="top:{top}%;width:{width}px;opacity:{opacity};animation-duration:{dur}s;animation-delay:{delay}s"
		><use href="#awan" /></svg>
	{/each}
</div>
