import { describe, expect, it } from 'vitest';
import { formatWait, invalidMessage, lockedMessage } from './lockout';

describe('formatWait', () => {
	it('memakai detik di bawah semenit dan membulatkan menit ke atas', () => {
		expect(formatWait(45)).toBe('45 detik');
		expect(formatWait(59.2)).toBe('1 menit');
		expect(formatWait(60)).toBe('1 menit');
		expect(formatWait(61)).toBe('2 menit');
		expect(formatWait(900)).toBe('15 menit');
	});
	it('tidak pernah menampilkan nol atau negatif', () => {
		expect(formatWait(0)).toBe('1 detik');
		expect(formatWait(-5)).toBe('1 detik');
	});
});

describe('invalidMessage', () => {
	const base = 'Email atau kata sandi salah.';
	it('menyebut sisa kesempatan dan lama kunci', () => {
		expect(invalidMessage(base, 3, 900)).toBe(`${base} Sisa 3 kesempatan sebelum akun dikunci selama 15 menit.`);
	});
	it('memperingatkan saat kesempatan habis', () => {
		expect(invalidMessage(base, 0, 900)).toBe(`${base} Percobaan berikutnya akan mengunci akun selama 15 menit.`);
	});
	it('tanpa info lama kunci tetap menulis "sementara"', () => {
		expect(invalidMessage(base, 2)).toBe(`${base} Sisa 2 kesempatan sebelum akun dikunci sementara.`);
	});
	it('tanpa info sisa kesempatan hanya pesan dasar', () => {
		expect(invalidMessage(base)).toBe(base);
	});
});

describe('lockedMessage', () => {
	it('menampilkan sisa waktu dan mengecil pada klik berikutnya', () => {
		expect(lockedMessage(840)).toContain('14 menit');
		expect(lockedMessage(779)).toContain('13 menit');
		expect(lockedMessage(30)).toContain('30 detik');
	});
	it('punya cadangan bila API tidak mengirim Retry-After', () => {
		expect(lockedMessage()).toContain('beberapa menit');
	});
});
