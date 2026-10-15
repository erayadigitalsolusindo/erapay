// Aksi review merchant (dipakai halaman antrean dan halaman detail). Izin dicek ulang oleh Go.
import { fail } from '@sveltejs/kit';
import { call, done, guard, int, str } from './gw';
import type { RequestEvent } from '@sveltejs/kit';

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
	// Tarif tidak punya nilai bawaan: keduanya wajib bilangan bulat 0..10000 (basis point, 100 = 1%).
	approve: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		const mdr = int(f, 'mdr_bps');
		const platform = int(f, 'platform_bps');
		if (![mdr, platform].every((n) => Number.isInteger(n) && n >= 0 && n <= 10000)) {
			return fail(400, { message: 'MDR dan fee platform harus bilangan bulat 0 sampai 10000 (basis point).' });
		}
		return done(await call(fetch, token, 'POST', path(f, 'approve'), { mdr_bps: mdr, platform_bps: platform }));
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
	// Verifikasi email pemilik secara manual (email tak sampai); alasan wajib dan tercatat di audit.
	verifyEmail: async ({ request, cookies, fetch, locals }: RequestEvent) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(f, 'verify-email'), { note: str(f, 'note') }));
	}
};
