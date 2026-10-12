import { error, fail, redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../../lib/server/auth';
import { getPayment, qrSvg } from '../../../../lib/server/payments';
import { call, done, int, newIdemKey, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, cookies, fetch, locals }) => {
	const token = cookies.get(SESSION_COOKIE);
	const payment = token ? await getPayment(fetch, token, params.id) : null;
	if (!payment) error(404, 'Pembayaran tidak ditemukan');
	// Go hanya mengisi qr_code selama status masih pending (masih bisa dibayar).
	const qr = payment.qr_code ? await qrSvg(payment.qr_code.value) : null;
	return {
		payment,
		qr,
		canRefund: !!locals.user?.merchantId && locals.user.permissions.includes('refund:request'),
		idem: newIdemKey('rf') // satu kunci per tampilan: submit ganda = satu refund
	};
};

export const actions: Actions = {
	refund: async ({ request, params, cookies, fetch, locals }) => {
		const token = cookies.get(SESSION_COOKIE);
		if (!token || !locals.user?.merchantId || !locals.user.permissions.includes('refund:request')) redirect(303, '/login');
		const f = await request.formData();
		const amount = int(f, 'amount');
		if (!Number.isSafeInteger(amount) || amount <= 0) return fail(400, { message: 'Nominal refund harus berupa angka rupiah penuh.' });
		const r = await call(fetch, token, 'POST', `/v1/payments/${encodeURIComponent(params.id)}/refund`, { amount, reason: str(f, 'reason') }, str(f, 'idem'));
		return done(r);
	}
};
