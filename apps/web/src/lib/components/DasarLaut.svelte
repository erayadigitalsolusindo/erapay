<script lang="ts">
	// Latar dasar laut: sinar cahaya, gelembung naik, ikan manta, dan ikan kecil yang berenang ke arah acak.
	// Semua dekoratif (aria-hidden); animasi dimatikan saat prefers-reduced-motion.

	type Opsi = { kecepatan: number };

	// Ikan bergerak: pilih titik acak di dalam induk, meluncur ke sana, balik badan sesuai arah, ulangi.
	function berenang(node: HTMLElement, { kecepatan }: Opsi) {
		const induk = node.parentElement as HTMLElement;
		const badan = node.firstElementChild as HTMLElement;
		const diam = matchMedia('(prefers-reduced-motion: reduce)').matches;
		let henti = false;
		let anim: Animation | undefined;
		const acak = (a: number, b: number) => a + Math.random() * (b - a);

		let x = acak(0, Math.max(1, induk.clientWidth - node.offsetWidth));
		let y = acak(induk.clientHeight * 0.1, induk.clientHeight * 0.85);
		node.style.transform = `translate(${x}px, ${y}px)`;

		async function jalan() {
			while (!henti && !diam) {
				const W = induk.clientWidth;
				const H = induk.clientHeight;
				const w = node.offsetWidth;
				const tx = acak(-w * 0.3, W - w * 0.7);
				const ty = acak(H * 0.06, H * 0.88);
				badan.style.transform = tx >= x ? 'scaleX(1)' : 'scaleX(-1)';
				const jarak = Math.hypot(tx - x, ty - y);
				anim = node.animate(
					[{ transform: `translate(${x}px, ${y}px)` }, { transform: `translate(${tx}px, ${ty}px)` }],
					{ duration: Math.max(1500, (jarak / kecepatan) * 1000), easing: 'ease-in-out', fill: 'forwards' }
				);
				try {
					await anim.finished;
				} catch {
					return;
				}
				x = tx;
				y = ty;
				node.style.transform = `translate(${x}px, ${y}px)`;
				anim.cancel();
				await new Promise((r) => setTimeout(r, acak(0, 900)));
			}
		}
		jalan();
		return {
			destroy() {
				henti = true;
				anim?.cancel();
			}
		};
	}

	// Gelembung: nilai tetap berbasis indeks supaya SSR dan klien sama.
	const gelembung = Array.from({ length: 30 }, (_, i) => ({
		kiri: (i * 37 + 11) % 100,
		ukuran: 6 + ((i * 13) % 18),
		durasi: 9 + ((i * 7) % 11),
		tunda: -((i * 5) % 17),
		goyang: 10 + ((i * 3) % 24)
	}));

	// Spesies: id gradien badan, warna sirip/ekor, hiasan tambahan (di dalam klip badan), lebar & kecepatan.
	const ikan = [
		{ g: 'badut', sirip: '#f06a12', ekor: '#f27a1c', hias: '<path d="M86 10v40M60 8v44M38 14v32" stroke="#fff" stroke-width="6"/><path d="M83 10v40M89 10v40M57 8v44M63 8v44M35 14v32M41 14v32" stroke="#1b1b1b" stroke-width="1.3" opacity=".7"/>', w: 'w-20', v: 52, tinggi: 1 },
		{ g: 'biru', sirip: '#1d4fb8', ekor: '#ffd21f', hias: '<path d="M96 22C70 16 50 24 36 30c14 4 34 10 60 6" fill="#0c1f4d" opacity=".85"/>', w: 'w-24', v: 46, tinggi: 1 },
		{ g: 'kuning', sirip: '#f3b90a', ekor: '#f7c81c', hias: '<path d="M50 30h50" stroke="#fff" stroke-opacity=".2" stroke-width="2"/>', w: 'w-20', v: 60, tinggi: 1 },
		{ g: 'kupu', sirip: '#ffe27a', ekor: '#fff0b0', hias: '<path d="M96 8l-6 44" stroke="#111" stroke-width="7"/><path d="M60 6c-4 16-4 32 0 48" stroke="#2b2b2b" stroke-width="3" fill="none" opacity=".6"/>', w: 'w-16', v: 66, tinggi: 1 },
		{ g: 'perak', sirip: '#9db4c2', ekor: '#b6c9d4', hias: '<path d="M30 28h72" stroke="#3a6a8a" stroke-width="4" opacity=".45"/>', w: 'w-14', v: 95, tinggi: 0.62 },
		{ g: 'perak', sirip: '#9db4c2', ekor: '#b6c9d4', hias: '<path d="M30 28h72" stroke="#3a6a8a" stroke-width="4" opacity=".45"/>', w: 'w-12', v: 105, tinggi: 0.62 },
		{ g: 'perak', sirip: '#9db4c2', ekor: '#b6c9d4', hias: '<path d="M30 28h72" stroke="#3a6a8a" stroke-width="4" opacity=".45"/>', w: 'w-14', v: 90, tinggi: 0.62 },
		{ g: 'kerapu', sirip: '#8a3b22', ekor: '#a24a2c', hias: '<g fill="#5a2312" opacity=".55"><circle cx="80" cy="22" r="3"/><circle cx="66" cy="34" r="3.5"/><circle cx="52" cy="24" r="2.8"/><circle cx="90" cy="38" r="2.4"/><circle cx="40" cy="34" r="2.6"/></g>', w: 'w-24', v: 38, tinggi: 1 }
	];
</script>

<div class="laut absolute inset-0 -z-10 overflow-hidden" aria-hidden="true">
	<!-- Definisi gradien bersama untuk semua ikan -->
	<svg width="0" height="0" class="absolute" focusable="false">
		<defs>
			<clipPath id="bd"><path d="M114 30C106 10 80 6 54 12 42 15 32 22 24 30c8 8 18 15 30 18 26 6 52 2 60-18Z" /></clipPath>
			<linearGradient id="vol" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#000" stop-opacity=".32" /><stop offset=".45" stop-color="#fff" stop-opacity=".06" /><stop offset=".8" stop-color="#fff" stop-opacity=".3" /><stop offset="1" stop-color="#fff" stop-opacity=".45" /></linearGradient>
			<linearGradient id="g-badut" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ff7a14" /><stop offset="1" stop-color="#ffa347" /></linearGradient>
			<linearGradient id="g-biru" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2f8cff" /><stop offset="1" stop-color="#1a4fc4" /></linearGradient>
			<linearGradient id="g-kuning" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ffe14d" /><stop offset="1" stop-color="#f5b800" /></linearGradient>
			<linearGradient id="g-kupu" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff6c8" /><stop offset="1" stop-color="#ffd84a" /></linearGradient>
			<linearGradient id="g-perak" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#6e93aa" /><stop offset=".5" stop-color="#cfe0ea" /><stop offset="1" stop-color="#f4fafc" /></linearGradient>
			<linearGradient id="g-kerapu" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#b8522e" /><stop offset="1" stop-color="#e59a6a" /></linearGradient>
			<radialGradient id="mantaBadan" cx=".55" cy=".5" r=".6"><stop offset="0" stop-color="#274f6b" /><stop offset=".6" stop-color="#173a52" /><stop offset="1" stop-color="#0d2535" /></radialGradient>
		</defs>
	</svg>

	<!-- Sinar cahaya dari permukaan -->
	<div class="sinar absolute -top-10 left-[8%] h-[120%] w-24 -rotate-12 bg-gradient-to-b from-white/20 to-transparent blur-md"></div>
	<div class="sinar absolute -top-10 left-[34%] h-[120%] w-32 -rotate-12 bg-gradient-to-b from-white/15 to-transparent blur-lg [animation-delay:-4s]"></div>
	<div class="sinar absolute -top-10 left-[66%] h-[120%] w-20 -rotate-12 bg-gradient-to-b from-white/20 to-transparent blur-md [animation-delay:-8s]"></div>
	<div class="sinar absolute -top-10 left-[88%] h-[120%] w-28 -rotate-12 bg-gradient-to-b from-white/10 to-transparent blur-lg [animation-delay:-2s]"></div>

	<!-- Gelembung -->
	{#each gelembung as g}
		<span
			class="gelembung absolute bottom-0 rounded-full border border-white/50 bg-white/10"
			style="left:{g.kiri}%;width:{g.ukuran}px;height:{g.ukuran}px;animation-duration:{g.durasi}s;animation-delay:{g.tunda}s;--goyang:{g.goyang}px"
		></span>
	{/each}

	<!-- Ikan -->
	{#each ikan as f}
		<div use:berenang={{ kecepatan: f.v }} class="absolute top-0 left-0 {f.w}">
			<div>
				<svg class="ikan block w-full" viewBox="0 0 120 60" style="transform:scaleY({f.tinggi})">
					<path class="ekor" d="M28 30C18 20 9 12 3 6c5 14 5 34 0 48 6-6 15-14 25-24Z" fill={f.ekor} opacity=".92" />
					<path d="M84 13C76-1 58 1 48 15Z" fill={f.sirip} opacity=".9" />
					<path d="M70 44c-4 8-10 12-18 12 2-6 6-10 12-14Z" fill={f.sirip} opacity=".85" />
					<g clip-path="url(#bd)">
						<rect width="120" height="60" fill="url(#g-{f.g})" />
						{@html f.hias}
						<rect width="120" height="60" fill="url(#vol)" />
					</g>
					<path d="M92 19c-4 6-4 16 0 22" stroke="#000" stroke-opacity=".28" stroke-width="1.6" fill="none" />
					<path d="M80 38c-6 5-12 7-18 6 4-5 10-8 18-6Z" fill={f.sirip} />
					<circle cx="100" cy="26" r="4.2" fill="#fff" />
					<circle cx="101" cy="26" r="2.6" fill="#10151c" />
					<circle cx="102" cy="25" r=".9" fill="#fff" />
				</svg>
			</div>
		</div>
	{/each}

	<!-- Ikan manta (tampak atas) -->
	<div use:berenang={{ kecepatan: 26 }} class="absolute top-0 left-0 w-48 sm:w-72">
		<div>
			<svg class="manta block w-full" viewBox="0 0 240 120">
				<path d="M12 60C60 56 90 58 104 59" stroke="#17384f" stroke-width="2.2" fill="none" stroke-linecap="round" />
				<path d="M214 60c-4-8-10-12-20-14 4 4 6 9 7 14-1 5-3 10-7 14 10-2 16-6 20-14Z" fill="#17384f" />
				<path d="M216 60C198 44 160 36 124 18 98 6 66 4 48 6c20 22 34 36 48 48l-10 6 10 6C82 78 68 92 48 114c18 2 50 0 76-12 36-18 74-26 92-42Z" fill="url(#mantaBadan)" />
				<path d="M216 60C198 44 160 36 124 18 98 6 66 4 48 6c20 22 34 36 48 48" fill="none" stroke="#7fb4d0" stroke-opacity=".35" stroke-width="1.5" />
				<path d="M196 50c8-2 14-2 18 2-5 3-11 4-18-2ZM196 70c8 2 14 2 18-2-5-3-11-4-18 2Z" fill="#102b3e" />
				<path d="M168 44c-8-4-18-5-26-3 2 8 8 14 16 14ZM168 76c-8 4-18 5-26 3 2-8 8-14 16-14Z" fill="#e8f4fa" opacity=".92" />
				<g fill="#e8f4fa" opacity=".5"><circle cx="120" cy="52" r="2.2" /><circle cx="108" cy="46" r="1.8" /><circle cx="118" cy="68" r="2.2" /><circle cx="106" cy="74" r="1.8" /><circle cx="94" cy="58" r="1.6" /><circle cx="94" cy="64" r="1.6" /></g>
				<circle cx="176" cy="40" r="1.8" fill="#02121c" /><circle cx="176" cy="80" r="1.8" fill="#02121c" />
			</svg>
		</div>
	</div>
</div>

<style>
	.sinar { animation: sinar 12s ease-in-out infinite alternate; transform-origin: top; }
	@keyframes sinar { from { opacity: .35; transform: rotate(-12deg) translateX(-20px); } to { opacity: 1; transform: rotate(-9deg) translateX(30px); } }
	.gelembung { animation: naik linear infinite; opacity: 0; will-change: transform; }
	@keyframes naik {
		0% { transform: translate(0, 0) scale(.6); opacity: 0; }
		10% { opacity: .9; }
		50% { transform: translate(var(--goyang), -50vh) scale(1); }
		100% { transform: translate(calc(var(--goyang) * -1), -110vh) scale(1.1); opacity: 0; }
	}
	.ekor { transform-origin: 28px 30px; animation: kibas .55s ease-in-out infinite alternate; }
	@keyframes kibas { from { transform: scaleX(1) skewY(-8deg); } to { transform: scaleX(.8) skewY(8deg); } }
	.manta { animation: kepak 2.4s ease-in-out infinite alternate; transform-origin: 50% 50%; }
	@keyframes kepak { from { transform: scaleY(1); } to { transform: scaleY(.78); } }
	@media (prefers-reduced-motion: reduce) {
		.sinar, .gelembung, .ekor, .manta { animation: none; }
		.gelembung { opacity: .5; bottom: auto; top: 30%; }
	}
</style>
