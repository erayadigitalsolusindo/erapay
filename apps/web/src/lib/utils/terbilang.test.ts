import { expect, test } from 'vitest';
import { terbilang } from './terbilang';

test('terbilang', () => {
	expect(terbilang(2000)).toBe('dua ribu');
	expect(terbilang(125000)).toBe('seratus dua puluh lima ribu');
	expect(terbilang(1500)).toBe('seribu lima ratus');
	expect(terbilang(11000)).toBe('sebelas ribu');
	expect(terbilang(1250000)).toBe('satu juta dua ratus lima puluh ribu');
});
