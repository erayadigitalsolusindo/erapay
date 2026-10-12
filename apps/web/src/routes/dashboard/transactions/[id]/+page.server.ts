import { error } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../../lib/server/auth';
import { getPayment, qrSvg } from '../../../../lib/server/payments';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, cookies, fetch }) => {
	const token = cookies.get(SESSION_COOKIE);
	const payment = token ? await getPayment(fetch, token, params.id) : null;
	if (!payment) error(404, 'Pembayaran tidak ditemukan');
	// Go hanya mengisi qr_code selama status masih pending (masih bisa dibayar).
	const qr = payment.qr_code ? await qrSvg(payment.qr_code.value) : null;
	return { payment, qr };
};
