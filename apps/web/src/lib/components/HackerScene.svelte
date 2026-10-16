<script lang="ts">
	// Ilustrasi animasi: hacker berhoodie mengetik di laptop, terminal holografik, hujan kode, gembok membuka-menutup.
	// Murni SVG + CSS (tanpa aset/JS); semua gerak dimatikan oleh prefers-reduced-motion.
	let { label = '', status = ['ENCRYPTING', 'ACCESS GRANTED'] }: { label?: string; status?: [string, string] | string[] } = $props();

	const COLS = [14, 40, 68, 96, 128, 160, 196, 226, 254, 284];
	const CH = ['1', '0', 'a', '7', 'f', '0', '1', '3', 'e', '9'];
</script>

<svg viewBox="0 0 300 340" class="scene size-full" role="img" aria-label={label} preserveAspectRatio="xMidYMid meet">
	<defs>
		<linearGradient id="hs-bg" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color="#14050a" />
			<stop offset="1" stop-color="#3a0a14" />
		</linearGradient>
		<radialGradient id="hs-glow" cx="0.75" cy="0.45" r="0.6">
			<stop offset="0" stop-color="#4ade80" stop-opacity="0.35" />
			<stop offset="1" stop-color="#4ade80" stop-opacity="0" />
		</radialGradient>
		<clipPath id="hs-type"><rect class="type-clip" x="150" y="86" width="0" height="12" /></clipPath>
		<clipPath id="hs-face"><circle cx="108" cy="142" r="22" /></clipPath>
	</defs>

	<rect width="300" height="340" fill="url(#hs-bg)" />
	<rect width="300" height="340" fill="url(#hs-glow)" />

	<!-- hujan kode -->
	<g class="rain" font-family="ui-monospace, Menlo, Consolas, monospace" font-size="11" fill="#4ade80" aria-hidden="true">
		{#each COLS as x, i (x)}
			<g class="col" style:animation-delay={`${-(i * 0.7) % 5}s`} style:animation-duration={`${3.6 + (i % 4) * 0.9}s`}>
				{#each [0, 1, 2, 3, 4, 5] as r (r)}
					<text {x} y={-10 + r * 14} opacity={0.15 + r * 0.14}>{CH[(i + r * 3) % CH.length]}</text>
				{/each}
			</g>
		{/each}
	</g>

	<!-- jendela terminal holografik -->
	<g class="hud float" aria-hidden="true">
		<rect x="132" y="34" width="156" height="118" rx="9" fill="#04120a" fill-opacity="0.82" stroke="#4ade80" stroke-opacity="0.65" />
		<rect x="132" y="34" width="156" height="17" rx="9" fill="#4ade80" fill-opacity="0.16" />
		<circle cx="145" cy="42.5" r="2.6" fill="#f87171" /><circle cx="155" cy="42.5" r="2.6" fill="#fbbf24" /><circle cx="165" cy="42.5" r="2.6" fill="#4ade80" />
		<!-- baris kode yang "ditulis" -->
		<rect class="code c1" x="144" y="60" width="92" height="4" rx="2" fill="#4ade80" />
		<rect class="code c2" x="152" y="69" width="70" height="4" rx="2" fill="#86efac" />
		<rect class="code c3" x="152" y="78" width="104" height="4" rx="2" fill="#4ade80" />
		<g clip-path="url(#hs-type)">
			<text x="144" y="96" font-family="ui-monospace, Menlo, Consolas, monospace" font-size="9.5" fill="#bbf7d0">$ argon2id --seal ••••••</text>
		</g>
		<rect class="cursor" x="144" y="106" width="6" height="10" fill="#4ade80" />
		<!-- bar progres -->
		<rect x="144" y="124" width="132" height="5" rx="2.5" fill="#4ade80" fill-opacity="0.18" />
		<rect class="progress" x="144" y="124" width="132" height="5" rx="2.5" fill="#4ade80" />
		<!-- status bergantian -->
		<text class="st st1" x="144" y="145" font-family="ui-monospace, Menlo, Consolas, monospace" font-size="8.5" fill="#fbbf24">{status[0]}…</text>
		<text class="st st2" x="144" y="145" font-family="ui-monospace, Menlo, Consolas, monospace" font-size="8.5" fill="#4ade80">✓ {status[1]}</text>
		<!-- gembok -->
		<g transform="translate(252 105)">
			<circle r="16" fill="#4ade80" fill-opacity="0.13" class="pulse" />
			<path class="shackle" d="M-6 -2v-5a6 6 0 0 1 12 0v5" fill="none" stroke="#4ade80" stroke-width="2.6" stroke-linecap="round" />
			<rect x="-9" y="-2" width="18" height="14" rx="3" fill="#4ade80" />
			<circle cy="5" r="2" fill="#04120a" />
		</g>
	</g>

	<!-- kursi + paha -->
	<rect x="34" y="176" width="9" height="92" rx="4" fill="#2a1a20" />
	<rect x="30" y="262" width="62" height="8" rx="4" fill="#2a1a20" />
	<rect x="46" y="296" width="6" height="34" rx="3" fill="#1f1317" />
	<rect x="68" y="250" width="72" height="17" rx="8" fill="#1e2a3f" />

	<!-- badan hoodie (menunduk ke laptop) -->
	<path d="M50 266 C46 214 60 178 90 168 L116 178 C104 204 108 234 116 266 Z" fill="#5a2535" />
	<path d="M90 168 L116 178 C106 198 106 220 110 244" fill="none" stroke="#4ade80" stroke-opacity="0.3" stroke-width="2" stroke-linecap="round" />

	<!-- lengan belakang -->
	<g class="arm arm-b">
		<path d="M96 182 L122 224 L170 226" fill="none" stroke="#4a1a2c" stroke-width="13" stroke-linecap="round" stroke-linejoin="round" />
		<circle cx="171" cy="226" r="6" fill="#c89c82" />
	</g>

	<!-- kepala + tudung -->
	<circle cx="105" cy="143" r="27" fill="#6e2c40" />
	<circle cx="108" cy="142" r="22" fill="#2e161c" />
	<g clip-path="url(#hs-face)">
		<ellipse cx="124" cy="143" rx="14" ry="19" fill="#b98a72" />
		<ellipse cx="128" cy="143" rx="12" ry="19" fill="#4ade80" fill-opacity="0.22" />
		<rect class="blink" x="123" y="137" width="7" height="2.8" rx="1.4" fill="#04120a" />
		<path d="M126 152 q5 3 9 -1" fill="none" stroke="#7a4f3d" stroke-width="1.6" stroke-linecap="round" />
	</g>
	<path d="M84 126 C92 108 124 106 132 128" fill="none" stroke="#4ade80" stroke-opacity="0.35" stroke-width="2" stroke-linecap="round" />
	<!-- tali hoodie -->
	<path d="M98 172 v18 M106 174 v14" stroke="#e8b84a" stroke-width="1.8" stroke-linecap="round" />

	<!-- lengan depan -->
	<g class="arm arm-f">
		<path d="M98 182 L126 228 L176 230" fill="none" stroke="#6e2c40" stroke-width="14" stroke-linecap="round" stroke-linejoin="round" />
		<circle cx="178" cy="230" r="6.5" fill="#d4a98e" />
	</g>

	<!-- meja -->
	<rect x="118" y="232" width="182" height="8" rx="3" fill="#5a2430" />
	<rect x="118" y="240" width="182" height="100" fill="#1c0a10" />
	<rect x="118" y="232" width="182" height="2" fill="#4ade80" fill-opacity="0.25" />

	<!-- laptop (layar menghadap pengguna; kita lihat sisinya + cahayanya) -->
	<path d="M150 232 L262 232 L268 226 L156 226 Z" fill="#3b3b44" />
	<path d="M224 226 L244 150 L252 152 L238 226 Z" fill="#2c2c34" />
	<path d="M224 226 L244 150 L252 152 L238 226 Z" fill="none" stroke="#4ade80" stroke-opacity="0.55" />
	<!-- cahaya layar yang jatuh ke wajah dan tangan -->
	<path d="M222 224 L120 168 L120 232 Z" fill="#4ade80" fill-opacity="0.07" />
	<!-- keyboard berkedip -->
	<g class="keys" fill="#4ade80" aria-hidden="true">
		<rect class="k k1" x="164" y="228.6" width="9" height="2.4" rx="1" />
		<rect class="k k2" x="178" y="228.6" width="9" height="2.4" rx="1" />
		<rect class="k k3" x="192" y="228.6" width="9" height="2.4" rx="1" />
		<rect class="k k4" x="206" y="228.6" width="9" height="2.4" rx="1" />
	</g>

	<!-- cangkir kopi + uap -->
	<rect x="272" y="212" width="16" height="20" rx="3" fill="#e8b84a" />
	<path d="M288 217 q7 0 7 6 q0 6 -7 6" fill="none" stroke="#e8b84a" stroke-width="2.4" />
	<path class="steam s1" d="M276 206 q-4 -6 0 -11 q4 -5 0 -10" fill="none" stroke="#fff" stroke-opacity="0.45" stroke-width="2" stroke-linecap="round" />
	<path class="steam s2" d="M283 206 q-4 -6 0 -11 q4 -5 0 -10" fill="none" stroke="#fff" stroke-opacity="0.45" stroke-width="2" stroke-linecap="round" />

	<!-- simbol melayang -->
	<g font-family="ui-monospace, Menlo, Consolas, monospace" font-size="13" font-weight="700" fill="#e8b84a" aria-hidden="true">
		<text class="sym a" x="22" y="90">{'{ }'}</text>
		<text class="sym b" x="64" y="52">&lt;/&gt;</text>
		<text class="sym c" x="40" y="120">0x</text>
	</g>
</svg>

<style>
	/* Hujan: tiap kolom jatuh lalu mengulang. */
	.col { animation: fall linear infinite; }
	@keyframes fall { from { transform: translateY(-20px); opacity: 0; } 15% { opacity: 1; } to { transform: translateY(330px); opacity: 0; } }

	.float { animation: hover 4s ease-in-out infinite; }
	@keyframes hover { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-5px); } }

	/* Baris kode: menulis (lebar tumbuh) lalu mengulang. */
	.code { transform-box: fill-box; transform-origin: left center; animation: write 5s ease-in-out infinite; }
	.c2 { animation-delay: 0.5s; }
	.c3 { animation-delay: 1s; }
	@keyframes write { 0% { transform: scaleX(0); } 22%, 80% { transform: scaleX(1); } 100% { transform: scaleX(0); } }

	/* Teks diketik huruf demi huruf. */
	.type-clip { animation: typing 5s steps(24, end) infinite; }
	@keyframes typing { 0% { width: 0; } 55%, 85% { width: 130px; } 100% { width: 0; } }
	.cursor { animation: blink 0.9s steps(1) infinite; }
	@keyframes blink { 50% { opacity: 0; } }

	.progress { transform-box: fill-box; transform-origin: left center; animation: load 5s ease-in-out infinite; }
	@keyframes load { 0% { transform: scaleX(0); } 70%, 90% { transform: scaleX(1); } 100% { transform: scaleX(1); opacity: 0; } }

	/* Status berganti: memproses → berhasil. */
	.st1 { animation: st1 5s infinite; }
	.st2 { opacity: 0; animation: st2 5s infinite; }
	@keyframes st1 { 0%, 68% { opacity: 1; } 72%, 100% { opacity: 0; } }
	@keyframes st2 { 0%, 70% { opacity: 0; } 76%, 94% { opacity: 1; } 100% { opacity: 0; } }

	/* Gembok: tertutup saat memproses, terbuka saat berhasil. */
	.shackle { animation: unlock 5s infinite; transform-box: fill-box; transform-origin: right bottom; }
	@keyframes unlock { 0%, 70% { transform: none; } 78%, 94% { transform: translateY(-4px) rotate(-26deg); } 100% { transform: none; } }
	.pulse { animation: pulse 2.5s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
	@keyframes pulse { 0%, 100% { transform: scale(1); opacity: 0.8; } 50% { transform: scale(1.25); opacity: 0.2; } }

	/* Mengetik: lengan bergantian naik-turun, tuts berkedip. */
	.arm { transform-box: fill-box; transform-origin: 20% 10%; }
	.arm-f { animation: type-f 0.34s ease-in-out infinite; }
	.arm-b { animation: type-b 0.34s ease-in-out infinite; animation-delay: -0.17s; }
	@keyframes type-f { 0%, 100% { transform: translateY(0) rotate(0); } 50% { transform: translateY(2.4px) rotate(0.9deg); } }
	@keyframes type-b { 0%, 100% { transform: translateY(0) rotate(0); } 50% { transform: translateY(2px) rotate(0.8deg); } }
	.k { opacity: 0.25; animation: key 0.7s steps(1) infinite; }
	.k2 { animation-delay: -0.17s; } .k3 { animation-delay: -0.35s; } .k4 { animation-delay: -0.52s; }
	@keyframes key { 0% { opacity: 1; } 35%, 100% { opacity: 0.25; } }

	.blink { animation: eye 4s infinite; transform-box: fill-box; transform-origin: center; }
	@keyframes eye { 0%, 94%, 100% { transform: scaleY(1); } 97% { transform: scaleY(0.1); } }

	.steam { stroke-dasharray: 26; animation: steam 2.6s ease-in-out infinite; }
	.s2 { animation-delay: -1.3s; }
	@keyframes steam { 0% { stroke-dashoffset: 26; opacity: 0; } 40% { opacity: 0.7; } 100% { stroke-dashoffset: -26; opacity: 0; } }

	.sym { animation: drift 5s ease-in-out infinite; }
	.sym.b { animation-delay: -1.6s; } .sym.c { animation-delay: -3.2s; }
	@keyframes drift { 0%, 100% { transform: translateY(0); opacity: 0.5; } 50% { transform: translateY(-8px); opacity: 1; } }

	@media (prefers-reduced-motion: reduce) {
		.col, .float, .code, .type-clip, .cursor, .progress, .st1, .st2, .shackle, .pulse, .arm-f, .arm-b, .k, .blink, .steam, .sym { animation: none !important; }
		.type-clip { width: 130px; }
		.st2 { opacity: 0; }
	}
</style>
