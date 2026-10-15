<script lang="ts">
	// Tombol "Aksi" + popup berisi semua tindakan review untuk satu merchant (sesuai status & izin).
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '$app/forms';
	import Modal from './Modal.svelte';
	import Combobox from './Combobox.svelte';
	import FeeInput from './FeeInput.svelte';
	import { getI18n } from '../i18n/i18n.svelte';
	import { notify } from '../ui/dialog.svelte';
	import { withConfirm } from '../ui/confirm';

	let {
		m,
		canReview,
		canApprove
	}: {
		m: { id: string; display_name: string; status: string; owner_email_verified: boolean };
		canReview: boolean;
		canApprove: boolean;
	} = $props();

	const { t } = getI18n();
	let open = $state(false);
	let err = $state('');
	let busy = $state(false);

	const reviewable = ['submitted', 'under_review', 'action_required'];
	const showVerify = $derived(canReview && !m.owner_email_verified);
	const showStart = $derived(canReview && m.status === 'submitted');
	const showChanges = $derived(canReview && ['submitted', 'under_review'].includes(m.status));
	const showDecide = $derived(canApprove && reviewable.includes(m.status));
	const showRevoke = $derived(canApprove && m.status === 'approved');
	const showReactivate = $derived(canApprove && m.status === 'suspended');
	const any = $derived(showVerify || showStart || showChanges || showDecide || showRevoke || showReactivate);

	// Ringkas isian tarif untuk dialog konfirmasi: "0,7%" atau "Rp 500".
	const feeText = (fd: FormData, key: string) => {
		const v = String(fd.get(`${key}_value`) ?? '').trim();
		return fd.get(`${key}_unit`) === 'rupiah' ? `Rp ${v}` : `${v}%`;
	};
	// Aksi tanpa dialog konfirmasi (mulai review, minta perbaikan, tolak sudah punya alasan wajib).
	const plain =
		(title: string, successMessage: string): SubmitFunction =>
		() => {
			busy = true;
			err = '';
			return async ({ result, update }) => {
				busy = false;
				if (result.type === 'failure') {
					err = (result.data as { message?: string })?.message ?? t('c.loadFail');
					notify('danger', err, `Gagal: ${title}`);
					return;
				}
				notify('success', successMessage, `Berhasil: ${title}`);
				await update();
				open = false;
			};
		};
	// Aksi berisiko: dialog konfirmasi dulu; hasil tampil sebagai toast.
	const busyWrap: SubmitFunction = () => {
		busy = true;
		err = '';
		return async ({ result, update }) => {
			busy = false;
			if (result.type === 'failure') return; // toast gagal sudah ditampilkan withConfirm
			await update();
			open = false;
		};
	};
</script>

{#if any}
	<button type="button" class="btn btn-red" onclick={() => ((open = true), (err = ''))}>
		<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8Z" /></svg>
		{t('c.actions')}
	</button>
{/if}

{#if open}
	<Modal title={`${t('c.actions')} · ${m.display_name}`} onclose={() => (open = false)} wide>
		<div class="space-y-5 text-xs">
			{#if err}<p class="rounded-lg bg-red-50 px-3 py-2 font-semibold text-red-700" role="alert">{err}</p>{/if}

			{#if showVerify}
				<form method="POST" action="?/verifyEmail" use:enhance={withConfirm({ variant: 'info', title: 'Verifikasi email manual', message: 'Tandai email pemilik sebagai terverifikasi secara manual? Alasanmu akan tercatat di audit.', confirmText: 'Ya, verifikasi', successMessage: 'Email pemilik ditandai terverifikasi.' }, busyWrap)} class="space-y-2">
					<h4 class="font-bold">Verifikasi email manual</h4>
					<p class="muted">{t('onb.emailNo')}. Gunakan bila email verifikasi tidak sampai; catat bukti verifikasinya.</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder="Alasan / bukti (mis. konfirmasi via telepon)" minlength="3" maxlength="500" required />
						<button class="btn" type="submit" disabled={busy}>Verifikasi email</button>
					</div>
				</form>
			{/if}

			{#if showStart}
				<form method="POST" action="?/startReview" use:enhance={plain('mulai review', 'Merchant kini berstatus Sedang ditinjau.')} class="flex items-center justify-between gap-3">
					<div><h4 class="font-bold">{t('onb.startReview')}</h4><p class="muted">Tandai bahwa kamu sedang meninjau data ini.</p></div>
					<input type="hidden" name="id" value={m.id} />
					<button class="btn" type="submit" disabled={busy}>{t('onb.startReview')}</button>
				</form>
			{/if}

			{#if showChanges}
				<form method="POST" action="?/requestChanges" use:enhance={plain('minta perbaikan', 'Catatan perbaikan sudah dikirim ke merchant.')} class="space-y-2">
					<h4 class="font-bold">{t('onb.requestChanges')}</h4>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('onb.changesNote')} minlength="3" maxlength="500" required />
						<button class="btn" type="submit" disabled={busy}>{t('onb.requestChanges')}</button>
					</div>
				</form>
			{/if}

			{#if showDecide}
				{#if !m.owner_email_verified}<p class="font-semibold text-amber-700">{t('onb.needVerify')}</p>{/if}
				<form
					method="POST"
					action="?/approve"
					use:enhance={withConfirm((fd) => ({ variant: 'info', title: t('onb.approveBtn'), message: t('onb.approveConfirm', { name: m.display_name, mdr: feeText(fd, 'mdr'), platform: feeText(fd, 'platform') }), confirmText: 'Ya, setujui', successMessage: 'Merchant disetujui. Tarif sudah ditetapkan.' }), busyWrap)}
					class="space-y-2"
				>
					<h4 class="font-bold">{t('onb.approveBtn')}</h4>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<FeeInput name="mdr" label="MDR (biaya gateway)" />
						<FeeInput name="platform" label="Fee platform EraPay" />
					</div>
					<div class="flex justify-end">
						<button class="btn btn-red" type="submit" disabled={busy}>{t('onb.approveBtn')}</button>
					</div>
					<p class="muted">{t('onb.bpsHint')}</p>
				</form>
				<form
					method="POST"
					action="?/reject"
					use:enhance={withConfirm((fd, action) => (action.search.includes('suspend')
						? { variant: 'warning', title: 'Tangguhkan merchant?', message: `Tangguhkan ${m.display_name}? Transaksi baru berhenti dan data terkunci sampai kamu mengaktifkannya kembali.`, confirmText: 'Ya, tangguhkan', successMessage: 'Merchant ditangguhkan.' }
						: { variant: 'danger', title: 'Tolak merchant?', message: `Tolak ${m.display_name}? Alasan penolakan akan dibaca merchant.`, confirmText: 'Ya, tolak', successMessage: 'Merchant ditolak.' }), busyWrap)}
					class="space-y-2"
				>
					<h4 class="font-bold">{t('fin.reject')} / Tangguhkan</h4>
					<p class="muted">Tolak = pengajuan ditutup (merchant boleh memperbaiki). Tangguhkan = transaksi dihentikan sementara dan data dikunci.</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('onb.noteRequired')} minlength="3" maxlength="500" required />
						<button class="btn btn-danger" type="submit" disabled={busy}>{t('fin.reject')}</button>
						<button class="btn btn-warn" type="submit" formaction="?/suspend" disabled={busy}>Tangguhkan</button>
					</div>
				</form>
			{/if}

			{#if showRevoke}
				<form method="POST" action="?/revoke" use:enhance={withConfirm((fd, action) => (action.search.includes('suspend')
						? { variant: 'warning', title: 'Tangguhkan merchant?', message: `Tangguhkan ${m.display_name}? Transaksi baru berhenti dan data terkunci sampai kamu mengaktifkannya kembali.`, confirmText: 'Ya, tangguhkan', successMessage: 'Merchant ditangguhkan.' }
						: { variant: 'danger', title: t('onb.revokeBtn'), message: t('onb.revokeConfirm', { name: m.display_name }), confirmText: 'Ya, cabut', successMessage: 'Persetujuan dicabut.' }), busyWrap)} class="space-y-2">
					<h4 class="font-bold">{t('onb.revokeBtn')}</h4>
					<p class="muted">{t('onb.revokeHint')}</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<Combobox id={`revoke-to-${m.id}`} name="to" class="w-44" required value="under_review" options={[
							{ value: 'under_review', label: t('onb.revokeToReview') },
							{ value: 'rejected', label: t('onb.revokeToRejected') }
						]} />
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('onb.noteRequired')} minlength="3" maxlength="500" required />
						<button class="btn btn-danger" type="submit" disabled={busy}>{t('onb.revokeBtn')}</button>
						<button class="btn btn-warn" type="submit" formaction="?/suspend" disabled={busy}>Tangguhkan</button>
					</div>
				</form>
			{/if}

			{#if showReactivate}
				<form
					method="POST"
					action="?/reactivate"
					use:enhance={withConfirm({ variant: 'info', title: 'Aktifkan kembali?', message: `Aktifkan kembali ${m.display_name}? Transaksi dapat berjalan lagi bila merchant sudah pernah disetujui.`, confirmText: 'Ya, aktifkan', successMessage: 'Merchant diaktifkan kembali.' }, busyWrap)}
					class="space-y-2"
				>
					<h4 class="font-bold">Aktifkan kembali</h4>
					<p class="muted">Merchant sedang ditangguhkan. Mengaktifkan kembali mengembalikannya ke status sebelumnya (disetujui bila pernah disetujui, jika tidak kembali ke peninjauan).</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder="Alasan pengaktifan kembali" minlength="3" maxlength="500" required />
						<button class="btn btn-red" type="submit" disabled={busy}>Aktifkan kembali</button>
					</div>
				</form>
			{/if}
		</div>
	</Modal>
{/if}
