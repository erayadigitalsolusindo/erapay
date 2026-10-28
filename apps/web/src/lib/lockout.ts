// Pesan kunci login: sisa kesempatan dan hitung mundur. Murni (tanpa $app/env) supaya mudah diuji.
// Aturan kunci ada di API (auth.Config: 5 gagal per email dalam jendela 15 menit); web hanya menampilkan.

/** "45 detik", "1 menit", "15 menit". Pembulatan ke atas supaya tidak pernah menjanjikan lebih cepat dari sebenarnya. */
export function formatWait(seconds: number): string {
	const s = Math.max(1, Math.ceil(seconds));
	if (s < 60) return `${s} detik`;
	return `${Math.ceil(s / 60)} menit`;
}

/** Pesan untuk kredensial salah: sisa kesempatan, dan berapa lama akun dikunci bila habis. */
export function invalidMessage(base: string, attemptsLeft?: number, lockoutSeconds?: number): string {
	if (attemptsLeft === undefined) return base;
	const lama = lockoutSeconds && lockoutSeconds > 0 ? ` selama ${formatWait(lockoutSeconds)}` : ' sementara';
	return attemptsLeft > 0
		? `${base} Sisa ${attemptsLeft} kesempatan sebelum akun dikunci${lama}.`
		: `${base} Percobaan berikutnya akan mengunci akun${lama}.`;
}

/** Pesan saat akun sedang terkunci; tiap klik ulang menunjukkan sisa waktu yang lebih kecil (dari API). */
export function lockedMessage(retryAfterSeconds?: number): string {
	return retryAfterSeconds && retryAfterSeconds > 0
		? `Terlalu banyak percobaan, akun dikunci sementara. Coba lagi dalam ${formatWait(retryAfterSeconds)}.`
		: 'Terlalu banyak percobaan, akun dikunci sementara. Coba lagi dalam beberapa menit.';
}
