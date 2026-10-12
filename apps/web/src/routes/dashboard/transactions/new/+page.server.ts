import { fail, redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../../lib/server/auth';
import { createQrisPayment } from '../../../../lib/server/payments';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals }) => {
	if (!locals.user?.permissions.includes('payment:create') || !locals.user.merchantId) redirect(303, '/dashboard');
	// Kunci idempotency dibuat saat form dibuka: kirim ulang (klik dobel/refresh) tidak membuat QR kedua.
	return { idempotencyKey: crypto.randomUUID() };
};

export const actions: Actions = {
	default: async ({ request, cookies, fetch }) => {
		const token = cookies.get(SESSION_COOKIE);
		if (!token) redirect(303, '/login');
		const form = await request.formData();
		const amountRaw = String(form.get('amount') ?? '').trim();
		const reference = String(form.get('reference') ?? '').trim();
		const description = String(form.get('description') ?? '').trim();
		const key = String(form.get('idempotencyKey') ?? '');
		const values = { amount: amountRaw, reference, description };

		if (!/^\d{1,12}$/.test(amountRaw) || Number(amountRaw) <= 0)
			return fail(400, { message: 'Nominal harus berupa bilangan bulat Rupiah, lebih dari 0.', values });
		if (!reference || reference.length > 64)
			return fail(400, { message: 'Referensi wajib diisi (maks. 64 karakter).', values });

		const res = await createQrisPayment(fetch, token, { merchantReference: reference, amount: Number(amountRaw), description }, key);
		if (!res.ok) return fail(res.payment ? 422 : 400, { message: res.message, values });
		redirect(303, `/dashboard/transactions/${res.payment.id}`);
	}
};
