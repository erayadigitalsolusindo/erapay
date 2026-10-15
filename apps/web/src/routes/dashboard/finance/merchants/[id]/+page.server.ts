import { redirect } from '@sveltejs/kit';
import { guard, read } from '../../../../../lib/server/gw';
import { merchantActions } from '../../../../../lib/server/merchantActions';
import type { Actions, PageServerLoad } from './$types';

// GET /v1/admin/merchants/{id} mengembalikan data diri dengan NIK/NPWP *plaintext* untuk reviewer
// (merchant:review). Halaman ini hanya untuk tampilan itu; daftar tetap memakai nilai ter-masking.
type Merchant = {
	id: string;
	legal_name: string;
	display_name: string;
	account_type: 'individual' | 'business' | null;
	owner_name: string | null;
	owner_email: string | null;
	owner_email_verified: boolean;
	contact_phone: string | null;
	status: string;
	review_note: string | null;
	created_at: string;
	identity_complete: boolean;
	identity: {
		id_masked?: string;
		id_number?: string;
		place_of_birth?: string;
		date_of_birth?: string;
		gender?: string;
		address?: string;
		npwp_masked?: string;
		npwp?: string;
		business_npwp_masked?: string;
		business_npwp?: string;
		business_nib?: string;
		complete: boolean;
	} | null;
};

// Profil KYC (data wizard merchant: perusahaan, perwakilan, merek, dokumen). File dilayani proksi
// /v1/admin/kyc-files/* (izin merchant:review dicek di Go).
export type KycFileRef = { id: string; url: string; name: string; mime: string; size: number };
type Kyc = {
	company: Record<string, string | boolean>;
	representative: Record<string, string> & { ktp?: KycFileRef | null; ktp_back?: KycFileRef | null };
	brand: {
		brand_id: string; name: string; category: string; description: string;
		monthly_amount: number; monthly_count: number;
		logo?: KycFileRef | null; links: { id: string; url: string }[]; evidences: KycFileRef[];
	};
	documents: Record<string, unknown>;
	submitted_at?: string;
	reviewed_at?: string;
	checklist: { section: string; label: string; complete: boolean; needs_revision: boolean }[];
	notes: { id: string; section: string; field?: string; note: string; resolved: boolean }[];
};

const F = '/v1/admin/kyc-files';
const fileOf = (d: any, kind: 'documents' | 'evidences'): KycFileRef | null =>
	d?.id ? { id: d.id, url: `${F}/${kind}/${d.id}`, name: d.original_name ?? d.name ?? '', mime: d.mime ?? '', size: d.size ?? 0 } : null;

export const load: PageServerLoad = async ({ locals, cookies, fetch, params }) => {
	const token = guard(locals, cookies, 'merchant:review', 'platform');
	const m = await read<Merchant>(fetch, token, `/v1/admin/merchants/${encodeURIComponent(params.id)}`);
	if (m === null) throw redirect(303, '/dashboard/finance/merchants');
	const kyc = await read<Kyc>(fetch, token, `/v1/admin/merchants/${encodeURIComponent(params.id)}/kyc`);
	let docs: { key: string; label: string; file: KycFileRef; number?: string }[] = [];
	if (kyc) {
		const d = kyc.documents as Record<string, any>;
		kyc.representative.ktp = fileOf(kyc.representative.ktp, 'documents');
		kyc.representative.ktp_back = fileOf(kyc.representative.ktp_back, 'documents');
		kyc.brand.evidences = (kyc.brand.evidences ?? []).map((e) => fileOf(e, 'evidences')!);
		kyc.brand.logo = kyc.brand.logo ? { ...kyc.brand.logo, id: 'logo', url: `${F}/logo/${params.id}` } : null;
		const spec: [string, string, string | undefined][] = [
			['npwp_badan', 'NPWP Badan Usaha', d.npwp_number],
			['akta', 'Akta Pendirian', undefined],
			['sk_kemenkumham', 'SK Kemenkumham', undefined],
			['nib', 'NIB / OSS', d.nib_number ? `${d.nib_number}${d.kbli ? ` · KBLI ${d.kbli}` : ''}` : undefined],
			['izin_usaha', 'Surat Izin Usaha', undefined]
		];
		docs = spec.flatMap(([key, label, number]) => {
			const file = fileOf(d[key], 'documents');
			return file ? [{ key, label, file, number }] : [];
		});
	}
	return {
		merchant: m,
		kyc,
		docs,
		canApprove: locals.user!.permissions.includes('merchant:approve'),
		canReview: locals.user!.permissions.includes('merchant:review')
	};
};

export const actions: Actions = merchantActions;
