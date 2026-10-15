// Aksi review merchant (dipakai halaman antrean dan halaman detail). Izin dicek ulang oleh Go.
import { fail } from '@sveltejs/kit';
import { call, done, guard, int, str } from './gw';
import type { RequestEvent } from '@sveltejs/kit';

const MAX_FIXED = 1_000_000; // sama dengan onboarding.MaxFixedFee di Go

/** Rupiah bulat dari isian ("100.000", "100000"); null bila bukan angka. */
function parseRupiah(raw: string): number | null {
	const digits = raw.trim().replace(/[.\s]/g, '').replace(/,\d+$/, '');
	return /^\d{1,12}$/.test(digits) ? Number(digits) : null;
}

function parseFee(f: FormData, key: string, label: string): { bps: number; fixed?: undefined } | { fixed: number; bps?: undefined } | { error: string } {
	const raw = str(f, `${key}_value`).trim();
	const unit = str(f, `${key}_unit`);
	if (!raw) return { error: `${label} wajib diisi.` };
	if (unit === 'rupiah') {
		const digits = raw.replace(/[.\s]/g, '').replace(/,\d+$/, '');
		if (!/^\d+$/.test(digits)) return { error: `${label}: isi nominal rupiah berupa angka bulat.` };
		const n = Number(digits);
		if (n > MAX_FIXED) return { error: `${label}: nominal maksimal Rp ${MAX_FIXED.toLocaleString('id-ID')}.` };
		return { fixed: n };
	}
	if (unit !== 'percent') return { error: `${label}: pilih satuan % atau Rp.` };
	const v = Number(raw.replace(',', '.'));
	if (!Number.isFinite(v) || v < 0 || v > 100) return { error: `${label}: persen harus antara 0 dan 100.` };
	const bps = Math.round(v * 100);
	if (Math.abs(bps - v * 100) > 1e-6) return { error: `${label}: persen maksimal 2 angka desimal (mis. 0,75).` };
	return { bps };
}

const path = (f: FormData, action: string) => `/v1/admin/merchants/${encodeURIComponent(str(f, 'id'))}/${action}`;

export const merchantActions = {
	startReview: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:review', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(f, 'start-review'), {}));
	},
	requestChanges: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:review', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(f, 'request-changes'), { note: str(f, 'note') }));
	},
	// Tarif tidak punya nilai bawaan. Tiap komponen (MDR, platform) diisi sebagai persen ATAU nominal rupiah:
	// persen -> basis point (0,01% ketelitian, 0..100%), rupiah -> bilangan bulat 0..MAX_FIXED.
	approve: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		const mdr = parseFee(f, 'mdr', 'MDR');
		if ('error' in mdr) return fail(400, { message: mdr.error });
		const platform = parseFee(f, 'platform', 'Fee platform');
		if ('error' in platform) return fail(400, { message: platform.error });
		// Aturan transaksi kecil (BI): transaksi sampai batas tertentu dikenai MDR flat. Opsional, diisi berpasangan.
		let flat: { mdr_flat_up_to: number; mdr_flat_fee: number } | undefined;
		if (f.get('flat_on') === 'on') {
			const upTo = parseRupiah(str(f, 'flat_up_to'));
			const fee = parseRupiah(str(f, 'flat_fee'));
			if (upTo === null || upTo <= 0) return fail(400, { message: 'Aturan transaksi kecil: isi batas transaksi (Rp) dengan angka bulat lebih dari 0.' });
			if (fee === null || fee > MAX_FIXED) return fail(400, { message: `Aturan transaksi kecil: isi biaya flat (Rp) angka bulat 0 sampai ${MAX_FIXED.toLocaleString('id-ID')}.` });
			if (fee > upTo) return fail(400, { message: 'Aturan transaksi kecil: biaya flat tidak boleh lebih besar dari batas transaksi.' });
			flat = { mdr_flat_up_to: upTo, mdr_flat_fee: fee };
		}
		return done(
			await call(fetch, token, 'POST', path(f, 'approve'), {
				...(mdr.bps !== undefined ? { mdr_bps: mdr.bps } : { mdr_fixed: mdr.fixed }),
				...(platform.bps !== undefined ? { platform_bps: platform.bps } : { platform_fixed: platform.fixed }),
				...flat
			})
		);
	},
	reject: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(f, 'reject'), { note: str(f, 'note') }));
	},
	// Cabut persetujuan: approved -> under_review | rejected. Alasan wajib (audit & dibaca merchant).
	revoke: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		const to = str(f, 'to');
		if (to !== 'under_review' && to !== 'rejected') return fail(400, { message: 'Pilih status tujuan.' });
		return done(await call(fetch, token, 'POST', path(f, 'revoke'), { to, note: str(f, 'note') }));
	},
	// Tangguhkan merchant (transaksi baru berhenti, data terkunci) dan aktifkan kembali; alasan wajib.
	suspend: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(f, 'suspend'), { note: str(f, 'note') }));
	},
	reactivate: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(f, 'reactivate'), { note: str(f, 'note') }));
	},
	// Verifikasi email pemilik secara manual (email tak sampai); alasan wajib dan tercatat di audit.
	verifyEmail: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:review', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(f, 'verify-email'), { note: str(f, 'note') }));
	}
};
