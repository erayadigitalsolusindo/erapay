// Pemanggilan /v1/payments di Go API memakai sesi dashboard. Validasi, fee, dan status
// seluruhnya ditentukan Go (PRD §0 #13); di sini hanya meneruskan dan menampilkan.
import QRCode from 'qrcode';
import { SESSION_COOKIE } from './auth';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';

export type PaymentStatus = 'created' | 'pending' | 'succeeded' | 'failed' | 'expired' | 'cancelled' | 'unknown';

export type Payment = {
	id: string;
	merchant_reference: string;
	description?: string;
	amount: number;
	currency: string;
	payment_method: string;
	status: PaymentStatus;
	gateway: string;
	qr_code?: { type: string; value: string };
	fee: { bearer: string; mdr_fee: number; platform_fee: number; merchant_net: number };
	expires_at: string | null;
	final_at: string | null;
	created_at: string;
	/** Status turunan, terpisah dari status pembayaran (PRD §0 #9). */
	settlement_status?: 'not_expected' | 'expected' | 'settled';
	refunded_amount?: number;
	refunding_amount?: number;
};

export type CreateResult =
	| { ok: true; payment: Payment }
	| { ok: false; message: string; payment?: Payment };

const headers = (token: string, extra: Record<string, string> = {}) => ({
	'content-type': 'application/json',
	cookie: `${SESSION_COOKIE}=${token}`,
	...extra
});

export async function createQrisPayment(
	f: typeof fetch,
	token: string,
	input: { merchantReference: string; amount: number; description: string },
	idempotencyKey: string
): Promise<CreateResult> {
	let res: Response;
	try {
		res = await f(`${BASE}/v1/payments`, {
			method: 'POST',
			headers: headers(token, { 'idempotency-key': idempotencyKey }),
			body: JSON.stringify({
				merchant_reference: input.merchantReference,
				amount: input.amount,
				currency: 'IDR',
				payment_method: 'QRIS',
				description: input.description || undefined
			})
		});
	} catch {
		return { ok: false, message: 'Layanan pembayaran tidak dapat dihubungi.' };
	}
	const body = await res.json().catch(() => null);
	if (res.ok || res.status === 202) return { ok: true, payment: body as Payment };
	if (res.status === 422 && body?.payment) {
		return { ok: false, message: 'Gateway menolak pembayaran ini.', payment: body.payment };
	}
	return { ok: false, message: body?.error?.message ?? 'Gagal membuat pembayaran.' };
}

export async function getPayment(f: typeof fetch, token: string, id: string): Promise<Payment | null> {
	try {
		const res = await f(`${BASE}/v1/payments/${encodeURIComponent(id)}`, { headers: headers(token) });
		return res.ok ? ((await res.json()) as Payment) : null;
	} catch {
		return null;
	}
}

/** QR dirender di server menjadi SVG; payload tidak pernah dikirim ke pihak ketiga. */
export const qrSvg = (value: string) => QRCode.toString(value, { type: 'svg', margin: 1, errorCorrectionLevel: 'M' });
