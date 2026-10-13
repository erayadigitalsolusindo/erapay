<!--
  Animasi laut untuk sidebar admin: tiga lapis ombak yang bergeser, pulau berayun
  dengan pohon swaying, manta & ikan yang berenang, dan gelembung naik. Port dari
  referensi docs/refrence/BatikSidebar.jsx ke Svelte 5 runes.

  murni dekoratif (aria-hidden) dan tidak pernah tampil kalau pengguna memilih
  reduced motion: kalau gerak dikurangi, bentuk tetap digambar diam tanpa rAF
  atau Web Animations, sesuai helper kurangGarak() yang dipakai halaman auth.
-->
<script lang="ts">
	import { onMount } from 'svelte';

	const NS = 'http://www.w3.org/2000/svg';
	const W = 268;

	let svg: SVGSVGElement;

	// dibuat ulang saat ukuran sidebar berubah; token mencegah build lama menimpa yang baru.
	onMount(() => {
		let raf = 0;
		let token = 0;
		let timer: ReturnType<typeof setTimeout>;

		const acak = (a: number, b: number) => a + Math.random() * (b - a);

		const ombak = (amp: number, puncak = false) => {
			let d = `M-134 0Q-100.5 ${-amp * 2} -67 0`;
			for (let i = 0; i < 7; i++) d += `T${i * 67} 0`;
			return puncak ? d : d + 'V3000H-134Z';
		};

		const buat = (tag: string, at: Record<string, string>, ke: Element) => {
			const e = document.createElementNS(NS, tag);
			for (const k in at) e.setAttribute(k, at[k]);
			ke.appendChild(e);
			return e;
		};

		const daun = () =>
			[-165, -130, -95, -60, -25, 15]
				.map((a) => `<path d="M0 0C8-9 22-9 30-1C20-5 10-4 0 0Z" transform="rotate(${a})" style="fill:var(--palm)"/>`)
				.join('');

		function bangun() {
			cancelAnimationFrame(raf);
			const mine = ++token;
			const diam = matchMedia('(prefers-reduced-motion: reduce)').matches;
			const h = svg.clientHeight || 800;
			const lvl = Math.max(h * 0.64, 430);
			svg.setAttribute('viewBox', `0 0 ${W} ${h}`);

			const goyang = (e: Element, a: string, b: string, d: number, asal?: string) => {
				if (diam) return;
				(e as SVGElement).style.transformOrigin = asal || '0 0';
				e.animate([{ transform: `rotate(${a})` }, { transform: `rotate(${b})` }], {
					duration: d,
					iterations: Infinity,
					direction: 'alternate',
					easing: 'ease-in-out'
				});
			};

			// nama, amplitudo,ofsset, opasitas, durasi geser, jarak geser, durasi ngambang
			const lapis: [string, number, number, number, number, number, number][] = [
				['back', 12, -18, 0.4, 11000, 134, 5200],
				['mid', 10, -8, 0.62, 7000, -134, 4300],
				['front', 8, 0, 0.92, 5000, 134, 3600]
			];
			const gambar = (l: (typeof lapis)[number]) =>
				`<g id="${l[0]}" transform="translate(0 ${lvl + l[2]})"><g id="${l[0]}i"><path d="${ombak(l[1])}" fill="url(#mg)" opacity="${l[3]}"/>` +
				(l[0] === 'mid'
					? `<path d="${ombak(l[1], true)}" fill="none" style="stroke:var(--foam)" stroke-width="2" stroke-linecap="round" opacity=".85"/>`
					: '') +
				`</g></g>`;

			svg.innerHTML =
				`<defs><linearGradient id="mg" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="420"><stop offset="0" style="stop-color:var(--w1)"/><stop offset="1" style="stop-color:var(--w2)"/></linearGradient></defs>` +
				gambar(lapis[0]) +
				`<g id="pulau" transform="translate(194 ${lvl - 4})"><path d="M-56 18C-48 -4 -24 -18 0 -18C24 -18 48 -4 56 18Z" style="fill:var(--sand)"/>` +
				`<path d="M-30 -2C-18 -14 -4 -17 8 -14C22 -17 34 -8 42 2C22 -4 0 -4 -30 -2Z" style="fill:var(--palm)" opacity=".8"/>` +
				`<g transform="translate(16 -14)"><g class="pl"><path d="M0 0C3 -14 8 -26 14 -36" fill="none" style="stroke:var(--trunk)" stroke-width="3" stroke-linecap="round"/><g transform="translate(14 -36)">${daun()}<circle cx="1" cy="3" r="2.2" style="fill:var(--trunk)"/></g></g></g>` +
				`<g transform="translate(-20 -10) scale(.65)"><g class="pl"><path d="M0 0C-2 -12 -6 -22 -12 -30" fill="none" style="stroke:var(--trunk)" stroke-width="3" stroke-linecap="round"/><g transform="translate(-12 -30)">${daun()}</g></g></g></g>` +
				gambar(lapis[1]) +
				gambar(lapis[2]) +
				`<g id="racun"></g><g id="gelembung"></g>`;

			const q = (id: string) => svg.querySelector('#' + id)!;

			if (!diam) {
				lapis.forEach((l) => {
					q(l[0] + 'i').animate([{ transform: 'translateX(0)' }, { transform: `translateX(${l[5]}px)` }], {
						duration: l[4],
						iterations: Infinity
					});
					q(l[0]).animate(
						[
							{ transform: `translate(0px,${lvl + l[2]}px)` },
							{ transform: `translate(0px,${lvl + l[2] + 9}px)` }
						],
						{ duration: l[6], iterations: Infinity, direction: 'alternate', easing: 'ease-in-out' }
					);
				});
				q('pulau').animate(
					[
						{ transform: `translate(194px,${lvl - 6}px)` },
						{ transform: `translate(194px,${lvl + 3}px)` }
					],
					{ duration: 4300, iterations: Infinity, direction: 'alternate', easing: 'ease-in-out' }
				);
				svg.querySelectorAll('.pl').forEach((p, i) => goyang(p, '-3deg', '4deg', 2600 + i * 700));
			}

			// manta & ikan: bergerak menuju target, belok halus di dekatnya
			const wadah = q('racun');
			const hidup: {
				e: SVGElement;
				s: number;
				vr: [number, number];
				x: number;
				y: number;
				vx: number;
				vy: number;
				f: number;
				sx: number;
				tx: number;
				ty: number;
				sp: number;
			}[] = [];
			const x0 = 18;
			const x1 = W - 18;
			const y0 = lvl + 36;
			const y1 = Math.max(h - 34, y0 + 1);

			const manta = (sk: number, op: number, sp: number) => {
				const e = buat('g', { opacity: String(op) }, wadah);
				e.innerHTML =
					`<g class="ek"><path d="M-30 2C-40 3-48 8-62 6C-48 0-40 -1-30 -1Z" style="fill:var(--cr)"/></g>` +
					`<g class="s2"><path d="M16 4C4 14-12 20-28 20C-16 12-8 8-4 2Z" style="fill:var(--cr)" opacity=".6"/></g>` +
					`<path d="M36 0C28-7 6-9-10-3C-18-1-26 0-32 2C-26 3-18 3-10 5C6 9 28 7 36 0Z" style="fill:var(--cr)"/>` +
					`<path d="M36 0l5-3-1 5zM34 2l5 2-4 1z" style="fill:var(--cr)"/>` +
					`<g class="s1"><path d="M16-4C4-16-16-24-34-22C-20-14-8-8-4-2Z" style="fill:var(--cr)"/></g><circle cx="27" cy="-2" r="1.2" style="fill:var(--foam)"/>`;
				goyang(e.querySelector('.s1')!, '-12deg', '16deg', 1300, '12px -4px');
				goyang(e.querySelector('.s2')!, '10deg', '-14deg', 1300, '12px 4px');
				goyang(e.querySelector('.ek')!, '-8deg', '8deg', 1500, '-30px 0px');
				hidup.push({ e, s: sk, vr: [sp * 0.6, sp], x: 0, y: 0, vx: 0, vy: 0, f: 1, sx: 1, tx: 0, ty: 0, sp: 0 });
			};

			const ikan = (merah: boolean, sk: number) => {
				const e = buat('g', {}, wadah);
				const c = merah ? 'var(--acc)' : 'var(--foam)';
				e.innerHTML =
					`<g class="ek"><path d="M-8 0L-18-6L-18 6Z" style="fill:${c}"/></g><path d="M-9 0C-5-6 5-6 12 0C5 6-5 6-9 0Z" style="fill:${c}"/>` +
					`<path d="M2-5L2 5" style="stroke:${merah ? 'var(--foam)' : 'var(--acc)'}" stroke-width="2"/><circle cx="8" cy="-1.5" r="1" style="fill:var(--ink)"/>`;
				goyang(e.querySelector('.ek')!, '-16deg', '16deg', acak(260, 380), '-8px 0px');
				hidup.push({ e, s: sk, vr: [34, 66], x: 0, y: 0, vx: 0, vy: 0, f: 1, sx: 1, tx: 0, ty: 0, sp: 0 });
			};

			manta(0.55, 0.55, 18);
			manta(0.9, 0.9, 24);
			for (let i = 0; i < 6; i++) ikan(i % 3 !== 2, acak(0.85, 1.15));

			const pilih = (a: (typeof hidup)[number]) => {
				for (let t = 0; t < 6; t++) {
					a.tx = acak(x0, x1);
					a.ty = acak(y0, y1);
					if (Math.hypot(a.tx - a.x, a.ty - a.y) > 90) break;
				}
				a.sp = acak(a.vr[0], a.vr[1]);
			};
			const gambarIkan = (a: (typeof hidup)[number]) => {
				const th = Math.max(-26, Math.min(26, Math.atan2(a.vy, Math.max(Math.abs(a.vx), 8)) * 57.3));
				a.e.setAttribute('transform', `translate(${a.x} ${a.y}) scale(${a.sx * a.s} ${a.s}) rotate(${th})`);
			};
			hidup.forEach((a) => {
				a.x = acak(x0, x1);
				a.y = acak(y0, y1);
				a.vx = acak(-20, 20);
				a.vy = 0;
				a.f = 1;
				a.sx = 1;
				pilih(a);
			});

			if (diam) {
				hidup.forEach(gambarIkan);
			} else {
				let last = performance.now();
				const frame = (t: number) => {
					if (mine !== token) return;
					const dt = Math.min((t - last) / 1000, 0.05);
					last = t;
					hidup.forEach((a) => {
						const dx = a.tx - a.x;
						const dy = a.ty - a.y;
						const d = Math.hypot(dx, dy) || 1;
						if (d < 26) pilih(a);
						const k = Math.min(1, dt * 1.1);
						a.vx += ((dx / d) * a.sp - a.vx) * k;
						a.vy += ((dy / d) * a.sp - a.vy) * k;
						a.x = Math.max(x0, Math.min(x1, a.x + a.vx * dt));
						a.y = Math.max(y0, Math.min(y1, a.y + a.vy * dt));
						if (Math.abs(a.vx) > 4) a.f = a.vx > 0 ? 1 : -1;
						a.sx += (a.f - a.sx) * Math.min(1, dt * 4);
						gambarIkan(a);
					});
					raf = requestAnimationFrame(frame);
				};
				raf = requestAnimationFrame(frame);
			}

			// gelembung naik; yang besar memuat motif bunga batik
			const gel = q('gelembung');
			for (let i = 0; i < 16; i++) {
				const r = 3 + acak(0, 10);
				const x = acak(14, W - 14);
				const atas = lvl - 50 - acak(0, 40);
				const dur = acak(6000, 12000);
				const sw = acak(8, 22);
				const e = buat('g', {}, gel);
				buat('circle', { r: String(r), style: 'fill:var(--foam);stroke:var(--foam)', 'fill-opacity': '0.13', 'stroke-opacity': '0.8', 'stroke-width': '1.2' }, e);
				buat('path', { d: `M${-r * 0.55} ${-r * 0.1}A${r * 0.6} ${r * 0.6} 0 0 1 ${-r * 0.1} ${-r * 0.58}`, fill: 'none', style: 'stroke:var(--foam)', 'stroke-width': '1.3', 'stroke-linecap': 'round', opacity: '0.9' }, e);
				if (r >= 8) {
					const f = buat('g', { opacity: '0.75' }, e);
					for (let k = 0; k < 4; k++) {
						buat('ellipse', { cy: String(-r * 0.3), rx: String(r * 0.13), ry: String(r * 0.28), fill: 'none', style: 'stroke:var(--foam)', 'stroke-width': '0.9', transform: `rotate(${k * 90})` }, f);
					}
					buat('circle', { r: '1.1', style: 'fill:var(--foam)' }, f);
				}
				const yb = h + 20;
				const tr = (t: number, dx: number, sc: number) => `translate(${x + dx}px,${yb + (atas - yb) * t}px) scale(${sc})`;
				if (diam) {
					(e as SVGElement).style.transform = tr(Math.random(), 0, 1);
					continue;
				}
				e.animate(
					[
						{ transform: tr(0, 0, 0.6), opacity: 0, offset: 0 },
						{ transform: tr(0.1, sw * 0.3, 0.8), opacity: 0.95, offset: 0.1 },
						{ transform: tr(0.35, sw, 0.95), opacity: 0.95, offset: 0.35 },
						{ transform: tr(0.65, -sw, 1), opacity: 0.95, offset: 0.65 },
						{ transform: tr(0.93, sw * 0.4, 1), opacity: 0.95, offset: 0.93 },
						{ transform: tr(1, 0, 1.35), opacity: 0, offset: 1 }
					],
					{ duration: dur, delay: -Math.random() * dur, iterations: Infinity, easing: 'linear' }
				);
			}
		}

		bangun();
		// sidebar tinggi penuh (h-screen) tapi tinggi viewport berubah saat jendela digeser.
		const ro = new ResizeObserver(() => {
			clearTimeout(timer);
			timer = setTimeout(bangun, 200);
		});
		ro.observe(svg);

		return () => {
			token++;
			cancelAnimationFrame(raf);
			clearTimeout(timer);
			ro.disconnect();
			svg.innerHTML = '';
		};
	});
</script>

<svg bind:this={svg} aria-hidden="true" class="sidebar-laut pointer-events-none absolute inset-0 h-full w-full"></svg>