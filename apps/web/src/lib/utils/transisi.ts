// Transisi Masuk <-> Daftar: dinding awan menutup layar dari dua sisi, halaman diganti,
// lalu awan menyibak. `semburUang` dipakai saat pendaftaran sukses. Hanya jalan di browser.

const NS = 'http://www.w3.org/2000/svg';
const WARNA_UANG = ['#c8102e', '#1d4ed8', '#15803d', '#7e22ce', '#ea580c', '#0e7490'];
const NOMINAL = ['2.000', '5.000', '10.000', '20.000', '50.000', '100.000'];

export const kurangGerak = () => matchMedia('(prefers-reduced-motion: reduce)').matches;

function bangunAwan(panel: HTMLElement, sisi: 'kiri' | 'kanan'): number {
	const W = Math.ceil(innerWidth * 0.62);
	const H = innerHeight;
	const langkah = Math.max(70, innerWidth / 20);
	const titik: [number, number, number][] = [];
	for (let y = -60; y < H + 120; y += langkah) {
		for (let x = -60; x < W + 60; x += langkah) {
			titik.push([
				x + (Math.random() - 0.5) * langkah * 0.7,
				y + (Math.random() - 0.5) * langkah * 0.7,
				langkah * 1.25 + Math.random() * langkah * 0.8
			]);
		}
	}
	// tepi yang menghadap tengah dibuat menggembung
	const tepi = sisi === 'kiri' ? W : 0;
	for (let y = -40; y < H + 80; y += langkah * 0.75) {
		titik.push([
			tepi + (Math.random() - 0.35) * 40 * (sisi === 'kiri' ? 1 : -1),
			y + (Math.random() - 0.5) * 30,
			langkah * 0.95 + Math.random() * langkah * 0.6
		]);
	}

	const svg = document.createElementNS(NS, 'svg');
	svg.setAttribute('width', String(W));
	svg.setAttribute('height', String(H));
	svg.style.cssText = 'display:block;overflow:visible';
	for (const [isi, dy, dr] of [['url(#gAwanBelakang)', 16, 6], ['url(#gAwan)', 0, 0]] as const) {
		const g = document.createElementNS(NS, 'g');
		g.setAttribute('fill', isi);
		for (const [cx, cy, r] of titik) {
			const c = document.createElementNS(NS, 'circle');
			c.setAttribute('cx', cx.toFixed(1));
			c.setAttribute('cy', (cy + dy).toFixed(1));
			c.setAttribute('r', (r + dr).toFixed(1));
			g.appendChild(c);
		}
		svg.appendChild(g);
	}
	panel.replaceChildren(svg);
	panel.style.width = W + 'px';
	panel.style.height = H + 'px';
	return W;
}

/** Jalankan animasi awan; `ganti` dipanggil saat layar tertutup penuh. */
export async function transisiAwan(kiri: HTMLElement, kanan: HTMLElement, ganti: () => Promise<void> | void) {
	const W = bangunAwan(kiri, 'kiri');
	bangunAwan(kanan, 'kanan');
	kiri.style.display = kanan.style.display = 'block';
	const geser = W + 240;
	try {
		const tutup = { duration: 750, easing: 'cubic-bezier(.55,.05,.25,1)', fill: 'forwards' } as const;
		await Promise.all([
			kiri.animate([{ transform: `translateX(${-geser}px)` }, { transform: 'translateX(0)' }], tutup).finished,
			kanan.animate([{ transform: `translateX(${geser}px)` }, { transform: 'translateX(0)' }], tutup).finished
		]);
		await ganti();
		scrollTo(0, 0);
		await new Promise((r) => setTimeout(r, 280));
		const buka = { duration: 850, easing: 'cubic-bezier(.45,0,.2,1)', fill: 'forwards' } as const;
		await Promise.all([
			kiri.animate([{ transform: 'translateX(0)' }, { transform: `translateX(${-geser}px)` }], buka).finished,
			kanan.animate([{ transform: 'translateX(0)' }, { transform: `translateX(${geser}px)` }], buka).finished
		]);
	} finally {
		kiri.style.display = kanan.style.display = 'none';
	}
}

// Uang mainan bergaya ERAPAY (bukan desain uang asli)
function elemenUang(): HTMLElement {
	const el = document.createElement('div');
	el.style.cssText = 'position:absolute;top:0;left:0;will-change:transform,opacity;filter:drop-shadow(0 6px 8px rgba(0,0,0,.35))';
	if (Math.random() < 0.3) {
		el.style.width = 34 + Math.random() * 24 + 'px';
		el.innerHTML = `<svg viewBox="0 0 32 32" width="100%"><circle cx="16" cy="16" r="14" fill="url(#emas)" stroke="#8a0a1f" stroke-width="1.5"/><circle cx="16" cy="16" r="10" fill="none" stroke="#8a0a1f" stroke-opacity=".5"/><text x="16" y="20.5" text-anchor="middle" font-size="12" font-weight="800" fill="#8a0a1f" font-family="Fraunces,serif">Rp</text></svg>`;
	} else {
		const c = WARNA_UANG[Math.floor(Math.random() * WARNA_UANG.length)];
		const n = NOMINAL[Math.floor(Math.random() * NOMINAL.length)];
		el.style.width = 110 + Math.random() * 90 + 'px';
		el.innerHTML = `<svg viewBox="0 0 200 100" width="100%"><rect width="200" height="100" rx="9" fill="${c}"/><rect x="6" y="6" width="188" height="88" rx="6" fill="none" stroke="#fff" stroke-opacity=".55" stroke-width="2"/><ellipse cx="100" cy="50" rx="22" ry="38" fill="none" stroke="#fff" stroke-opacity=".3" stroke-width="2"/><ellipse cx="100" cy="50" rx="38" ry="22" fill="none" stroke="#fff" stroke-opacity=".3" stroke-width="2"/><circle cx="100" cy="50" r="17" fill="#fff" fill-opacity=".2"/><text x="100" y="57" text-anchor="middle" font-size="20" font-weight="800" fill="#fff" font-family="Fraunces,serif">E</text><text x="16" y="28" font-size="13" font-weight="800" fill="#fff" font-family="Plus Jakarta Sans,sans-serif">${n}</text><text x="184" y="86" text-anchor="end" font-size="13" font-weight="800" fill="#fff" font-family="Plus Jakarta Sans,sans-serif">Rp</text></svg>`;
	}
	return el;
}

/** arah -1 = uang naik dari bawah, +1 = uang turun dari atas */
export function semburUang(lapisan: HTMLElement, arah: -1 | 1, jumlah: number) {
	const W = innerWidth;
	const H = innerHeight;
	for (let i = 0; i < jumlah; i++) {
		const el = elemenUang();
		lapisan.appendChild(el);
		const x = Math.random() * (W + 80) - 40;
		const dx = (Math.random() - 0.5) * 260;
		const y0 = arah < 0 ? H + 160 : -200;
		const y1 = arah < 0 ? -240 : H + 160;
		const r0 = (Math.random() - 0.5) * 80;
		const r1 = r0 + (Math.random() - 0.5) * 720;
		const s = 0.8 + Math.random() * 0.5;
		const anim = el.animate(
			[
				{ transform: `translate(${x}px,${y0}px) rotate(${r0}deg) scale(${s})`, opacity: 0 },
				{ opacity: 1, offset: 0.1 },
				{ opacity: 1, offset: 0.9 },
				{ transform: `translate(${x + dx}px,${y1}px) rotate(${r1}deg) scale(${s})`, opacity: 0 }
			],
			{ duration: 1300 + Math.random() * 800, delay: Math.random() * 550, easing: 'cubic-bezier(.35,.55,.4,1)' }
		);
		anim.onfinish = () => el.remove();
	}
}
