<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { enhance, deserialize } from '$app/forms';

	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import KycWizard from '../../../lib/components/kyc/KycWizard.svelte';
	import StatusAkun from '../../../lib/components/kyc/StatusAkun.svelte';
	import type { Profile, Region } from '../../../lib/server/kyc';
	import type { SectionApi } from '../../../lib/components/kyc/types';

	let { data, form }: { data: { profile: Profile | null; email: string; emailVerified: boolean }; form?: any } = $props();
	const i18n = getI18n();
	const { t } = i18n;

	// profil diperbarui oleh invalidateAll() setelah setiap aksi sukses (status KYC berubah)
	const profile = $derived(data.profile);
	const editable = $derived(!!profile?.editable && !profile?.locked);

	// —— Action helper: panggil server action (+page.server.ts) lalu muat ulang data page ——
	async function postAction(name: string, body?: unknown, isMultipart = false): Promise<any> {
		let res: Response;
		// x-sveltekit-action + accept JSON: tanpa ini SvelteKit membalas HTML (bukan JSON) sehingga
		// hasil aksi terbaca salah ("sukses" padahal tidak tersimpan) atau memicu 500 saat render.
		const headers: Record<string, string> = { accept: 'application/json', 'x-sveltekit-action': 'true' };
		try {
			if (isMultipart && body instanceof FormData) {
				res = await fetch(`?/${name}`, { method: 'POST', headers, body });
			} else {
				res = await fetch(`?/${name}`, {
					method: 'POST',
					headers,
					body: new URLSearchParams({ json: JSON.stringify(body ?? {}) })
				});
			}
		} catch {
			return { ok: false, message: 'Layanan tidak dapat dihubungi.' };
		}
		let result: any;
		try {
			result = deserialize(await res.text());
		} catch {
			return { ok: false, message: 'Respons server tidak valid.', status: res.status };
		}
		if (result.type === 'failure') {
			const d: any = result.data ?? {};
			return { ok: false, message: d.message ?? 'Permintaan gagal.', status: result.status, fields: d.fields, code: d.code };
		}
		if (result.type !== 'success') return { ok: false, message: 'Terjadi kesalahan server.', status: res.status };
		return { ok: true, ...((result.data as object) ?? {}) };
	}

	// Membangun SectionApi yang memakai action server (token & cookie sesi tidak pernah
	// bocor ke browser: semua panggilan Go lewat +page.server.ts).
	function buildApi(prof: Profile): SectionApi {
		return {
			save: async (section, payload) => {
				let r = await postAction('kycSave', { section, payload });
				// Akun sudah disetujui: ubah data sensitif (jenis entitas, nama badan, NIK) butuh konfirmasi eksplisit.
				if (!r.ok && r.code === 'SENSITIVE_CONFIRM_REQUIRED') {
					const yes = window.confirm(
						'Akun ini sudah terverifikasi. Mengubah data sensitif (mis. jenis entitas, nama badan usaha, NIK) akan menandai akun "Perlu Tinjau Ulang" dan menunggu persetujuan admin lagi. ' + 'Lanjutkan menyimpan?'
					);
					if (yes) r = await postAction('kycSave', { section, payload, confirm_sensitive: true });
				}
				if (r.ok) void invalidateAll();
				return { ok: r.ok, fields: r.fields, message: r.message };
			},
			submit: async () => {
				const r = await postAction('kycSubmit');
				if (r.ok) void invalidateAll();
				return { ok: r.ok, fields: r.fields, message: r.message };
			},
			uploadDoc: async (docType, file, extra) => {
				const fd = new FormData();
				fd.append('file', file);
				fd.append('doc_type', docType);
				if (extra?.number) fd.append('number', extra.number);
				if (extra?.kbli) fd.append('kbli', extra.kbli);
				if (extra?.confirm) fd.append('confirm', 'true');
				const r = await postAction('kycUpload', fd, true);
				if (r.ok) void invalidateAll();
				return { ok: r.ok, fields: r.fields, message: r.message };
			},
			uploadLogo: async (file) => {
				const fd = new FormData();
				fd.append('file', file);
				const r = await postAction('kycLogo', fd, true);
				if (r.ok) void invalidateAll();
				return { ok: r.ok, message: r.message };
			},
			addEvidence: async (file) => {
				const fd = new FormData();
				fd.append('file', file);
				const r = await postAction('kycEvidence', fd, true);
				if (r.ok) await invalidateAll();
				return { ok: r.ok, message: r.message };
			},
			deleteEvidence: async (id) => {
				const r = await postAction('kycDeleteEvidence', { id });
				if (r.ok) void invalidateAll();
				return { ok: r.ok, message: r.message };
			},
			regions: async (q) => {
				const r = await postAction('kycRegions', { q });
				return r.ok ? (r.regions as Region[]) ?? [] : [];
			}
		};
	}
	const api = $derived(profile ? buildApi(profile) : null);
</script>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
{#if form?.resent}<p class="mb-2 text-xs font-semibold text-emerald-800" role="status">{t('onb.acct.resent')}</p>{/if}

{#if !data.emailVerified}
	<section class="card p-5">
		<h2 class="text-sm font-bold">{t('onb.acct.email')}</h2>
		<p class="mt-2 text-xs">{t('onb.acct.emailPending', { email: data.email })}</p>
		<form method="POST" action="?/resend" class="mt-3" use:enhance>
			<button class="btn btn-red" type="submit">{t('onb.acct.resend')}</button>
		</form>
	</section>
{/if}

<!-- Wizard KYC saat editable (draft/action_required/rejected) memakai KycWizard -->
{#if api && profile}
	<KycWizard {profile} {api} />
{:else if profile}
	<!-- StatusAkun read-only untuk submitted/under_review/approved -->
	<StatusAkun {profile} />
{:else}
	<section class="card p-5">
		<h2 class="text-sm font-bold">{t('onb.acct.status')}</h2>
		<p class="mt-3 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	</section>
{/if}
