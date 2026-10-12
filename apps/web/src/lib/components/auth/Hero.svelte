<script lang="ts">
	import { getI18n } from '../../i18n/i18n.svelte';
	import Logo from './Logo.svelte';

	const { t } = getI18n();
</script>

<section class="hidden [text-shadow:0_2px_14px_rgb(74_5_16/.55)] lg:block">
	<div class="animate-naik inline-flex items-center gap-3">
		<Logo class="h-16 drop-shadow-lg" />
	</div>

	<h1 class="animate-naik font-display mt-6 text-4xl leading-[1.1] font-extrabold [animation-delay:.1s] xl:text-5xl">
		{t('auth.hero.l1')} <span class="teks-bendera">{t('auth.hero.pride')}</span>,<br />
		{t('auth.hero.l2')}<br />{t('auth.hero.l3')}
	</h1>

	<div class="garis-emas animate-kilau mt-6 h-0.5 w-48 rounded-full"></div>

	<div class="animate-naik relative mt-6 [animation-delay:.3s]">
		<svg viewBox="0 0 800 470" class="w-full max-w-2xl drop-shadow-[0_20px_40px_rgb(0_0_0/.3)]" role="img" aria-label={t('auth.hero.alt')}>
			<ellipse cx="400" cy="446" rx="380" ry="13" fill="#000" fill-opacity=".25" />

			<rect x="432" y="150" width="10" height="272" fill="#8a0a1f" />
			<rect x="748" y="150" width="10" height="272" fill="#8a0a1f" />
			<rect x="445" y="185" width="300" height="150" rx="6" fill="#fff8f0" fill-opacity=".16" />
			<rect x="460" y="252" width="170" height="6" rx="3" fill="#e8b84a" fill-opacity=".8" />
			<g>
				<rect x="470" y="226" width="22" height="26" rx="4" fill="#e8b84a" />
				<rect x="500" y="234" width="18" height="18" rx="4" fill="#fff8f0" />
				<rect x="560" y="228" width="24" height="24" rx="4" fill="#fff8f0" />
				<rect x="592" y="236" width="18" height="16" rx="4" fill="#e8b84a" />
			</g>
			<rect x="425" y="120" width="50" height="40" fill="#c8102e" /><circle cx="450" cy="160" r="25" fill="#c8102e" /><rect x="475" y="120" width="50" height="40" fill="#fff8f0" /><circle cx="500" cy="160" r="25" fill="#fff8f0" /><rect x="525" y="120" width="50" height="40" fill="#c8102e" /><circle cx="550" cy="160" r="25" fill="#c8102e" /><rect x="575" y="120" width="50" height="40" fill="#fff8f0" /><circle cx="600" cy="160" r="25" fill="#fff8f0" /><rect x="625" y="120" width="50" height="40" fill="#c8102e" /><circle cx="650" cy="160" r="25" fill="#c8102e" /><rect x="675" y="120" width="50" height="40" fill="#fff8f0" /><circle cx="700" cy="160" r="25" fill="#fff8f0" /><rect x="725" y="120" width="50" height="40" fill="#c8102e" /><circle cx="750" cy="160" r="25" fill="#c8102e" />
			<rect x="415" y="106" width="370" height="16" rx="8" fill="#8a0a1f" />

			<path d="M648,340 Q652,285 680,280 Q708,285 712,340 Z" fill="#e8b84a" />
			<circle cx="680" cy="208" r="11" fill="#2a1a12" />
			<circle cx="680" cy="246" r="26" fill="#d9a066" />
			<path d="M654,246 Q654,214 680,214 Q706,214 706,246 Q694,231 680,231 Q666,231 654,246 Z" fill="#2a1a12" />
			<circle cx="671" cy="249" r="2.3" fill="#3a2415" /><circle cx="689" cy="249" r="2.3" fill="#3a2415" />
			<path d="M672,260 Q680,267 688,260" fill="none" stroke="#7a3b1e" stroke-width="2.2" stroke-linecap="round" />

			<rect x="232" y="400" width="16" height="42" rx="6" fill="#2b2d42" />
			<rect x="252" y="400" width="16" height="42" rx="6" fill="#2b2d42" />
			<ellipse cx="240" cy="444" rx="13" ry="5" fill="#111" /><ellipse cx="260" cy="444" rx="13" ry="5" fill="#111" />
			<path d="M222,300 Q250,283 278,300 L284,405 L216,405 Z" fill="#fff8f0" />
			<g fill="#c8102e">
				<rect x="244" y="322" width="12" height="12" transform="rotate(45 250 328)" />
				<rect x="244" y="348" width="12" height="12" transform="rotate(45 250 354)" />
				<rect x="244" y="374" width="12" height="12" transform="rotate(45 250 380)" />
			</g>
			<path d="M224,306 L213,372" stroke="#fff8f0" stroke-width="16" stroke-linecap="round" fill="none" />
			<circle cx="212" cy="378" r="8" fill="#c68642" />
			<path d="M276,308 L360,288" stroke="#fff8f0" stroke-width="18" stroke-linecap="round" fill="none" />
			<circle cx="366" cy="286" r="9" fill="#c68642" />
			<circle cx="250" cy="252" r="28" fill="#c68642" />
			<path d="M222,240 Q250,206 278,240 L278,246 Q250,238 222,246 Z" fill="#1f1f1f" />
			<circle cx="241" cy="257" r="2.4" fill="#3a2415" /><circle cx="259" cy="257" r="2.4" fill="#3a2415" />
			<path d="M242,268 Q250,275 258,268" fill="none" stroke="#6b3a1a" stroke-width="2.2" stroke-linecap="round" />
			<g transform="rotate(8 375 285)">
				<rect x="358" y="255" width="34" height="60" rx="7" fill="#1f2937" />
				<rect x="362" y="260" width="26" height="50" rx="4" fill="#fff8f0" />
				<rect x="369" y="276" width="12" height="12" rx="2" fill="none" stroke="#c8102e" stroke-width="2" />
				<rect x="372" y="279" width="6" height="6" fill="#c8102e" />
			</g>

			<path d="M394,272 Q452,246 502,292" fill="none" stroke="#e8b84a" stroke-width="3" stroke-dasharray="6 8" stroke-linecap="round" class="animate-alur" />

			<rect x="430" y="335" width="340" height="87" rx="8" fill="#8a0a1f" />
			<rect x="424" y="326" width="352" height="14" rx="7" fill="url(#emas)" />
			<g fill="#e8b84a" fill-opacity=".75"><circle cx="470" cy="382" r="5" /><circle cx="520" cy="382" r="5" /><circle cx="570" cy="382" r="5" /><circle cx="620" cy="382" r="5" /><circle cx="670" cy="382" r="5" /><circle cx="720" cy="382" r="5" /></g>

			<rect x="495" y="270" width="50" height="58" rx="6" fill="#fff8f0" stroke="#8a0a1f" stroke-width="2" />
			<g fill="#4a0510">
				<rect x="502" y="277" width="13" height="13" /><rect x="525" y="277" width="13" height="13" /><rect x="502" y="298" width="13" height="13" />
				<rect x="505" y="280" width="7" height="7" fill="#fff8f0" /><rect x="528" y="280" width="7" height="7" fill="#fff8f0" /><rect x="505" y="301" width="7" height="7" fill="#fff8f0" />
				<rect x="525" y="298" width="5" height="5" /><rect x="533" y="302" width="5" height="5" /><rect x="525" y="306" width="5" height="5" /><rect x="518" y="294" width="4" height="4" />
			</g>
			<text x="520" y="324" text-anchor="middle" font-size="6.5" font-weight="800" fill="#c8102e" font-family="Plus Jakarta Sans, sans-serif" letter-spacing="1">ERAPAY</text>

			<rect x="598" y="306" width="44" height="22" rx="4" fill="#1f2937" />
			<rect x="603" y="310" width="34" height="9" rx="2" fill="#4ade80" />

			<g class="animate-terbang">
				<rect x="240" y="62" width="190" height="60" rx="16" fill="#fff8f0" stroke="#e8b84a" stroke-width="2" />
				<circle cx="268" cy="92" r="15" fill="#16a34a" />
				<path d="M261,92 l5,6 l10,-12" fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" />
				<text x="292" y="88" font-size="11.5" font-weight="700" fill="#4a0510" font-family="Plus Jakarta Sans, sans-serif">Pembayaran berhasil</text>
				<text x="292" y="108" font-size="15" font-weight="800" fill="#c8102e" font-family="Fraunces, serif">Rp25.000</text>
			</g>

			{#each [[120, 210, 0], [170, 140, -2], [70, 320, -4]] as [cx, cy, delay]}
				<g class="animate-terbang" style="animation-delay:{delay}s">
					<circle {cx} {cy} r="15" fill="url(#emas)" stroke="#8a0a1f" stroke-width="1.5" />
					<text x={cx} y={cy + 4} text-anchor="middle" font-size="11" font-weight="800" fill="#8a0a1f" font-family="Plus Jakarta Sans, sans-serif">Rp</text>
				</g>
			{/each}
		</svg>
	</div>
</section>
