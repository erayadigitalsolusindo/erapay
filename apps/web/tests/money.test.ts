import { describe, expect, it } from 'vitest';
import { formatIDR } from '../src/lib/utils/money';

describe('formatIDR', () => {
	it('memformat integer rupiah', () => {
		expect(formatIDR(1000).replace(/\s/g, ' ')).toContain('1.000');
	});
	it('menolak pecahan', () => {
		expect(() => formatIDR(10.5)).toThrow();
	});
});
