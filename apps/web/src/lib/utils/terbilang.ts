const S = ['', 'satu', 'dua', 'tiga', 'empat', 'lima', 'enam', 'tujuh', 'delapan', 'sembilan', 'sepuluh', 'sebelas'];

/** Angka bulat >= 0 menjadi kata Indonesia (untuk dibacakan suara), mis. 125000 -> "seratus dua puluh lima ribu". */
export function terbilang(n: number): string {
	if (n === 0) return 'nol';
	const w = (x: number): string => {
		if (x < 12) return S[x];
		if (x < 20) return `${S[x - 10]} belas`;
		if (x < 100) return `${S[Math.floor(x / 10)]} puluh ${S[x % 10]}`;
		if (x < 200) return `seratus ${w(x - 100)}`;
		if (x < 1000) return `${S[Math.floor(x / 100)]} ratus ${w(x % 100)}`;
		if (x < 2000) return `seribu ${w(x - 1000)}`;
		if (x < 1e6) return `${w(Math.floor(x / 1000))} ribu ${w(x % 1000)}`;
		if (x < 1e9) return `${w(Math.floor(x / 1e6))} juta ${w(x % 1e6)}`;
		return `${w(Math.floor(x / 1e9))} miliar ${w(x % 1e9)}`;
	};
	return w(Math.floor(n)).replace(/\s+/g, ' ').trim();
}
