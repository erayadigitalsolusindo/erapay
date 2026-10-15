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
	let flatOn = $state(false);

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
	const flatText = (fd: FormData) =>
		fd.get('flat_on') === 'on' ? t('ma.flat.summary', { upTo: String(fd.get('flat_up_to') ?? '').trim(), fee: String(fd.get('flat_fee') ?? '').trim() }) : '';
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
					notify('danger', err, t('ma.fail', { title }));
					return;
				}
				notify('success', successMessage, t('ma.success', { title }));
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
				<form method="POST" action="?/verifyEmail" use:enhance={withConfirm({ variant: 'info', title: t('ma.verify.title'), message: t('ma.verify.msg'), confirmText: t('ma.verify.ok'), successMessage: t('ma.verify.done') }, busyWrap)} class="space-y-2">
					<h4 class="font-bold">{t('ma.verify.title')}</h4>
					<p class="muted">{t('onb.emailNo')}. {t('ma.verify.hint')}</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('ma.verify.ph')} minlength="3" maxlength="500" required />
						<button class="btn" type="submit" disabled={busy}>{t('ma.verify.btn')}</button>
					</div>
				</form>
			{/if}

			{#if showStart}
				<form method="POST" action="?/startReview" use:enhance={plain(t('ma.start.action'), t('ma.start.done'))} class="flex items-center justify-between gap-3">
					<div><h4 class="font-bold">{t('onb.startReview')}</h4><p class="muted">{t('ma.start.hint')}</p></div>
					<input type="hidden" name="id" value={m.id} />
					<button class="btn" type="submit" disabled={busy}>{t('onb.startReview')}</button>
				</form>
			{/if}

			{#if showChanges}
				<form method="POST" action="?/requestChanges" use:enhance={plain(t('ma.changes.action'), t('ma.changes.done'))} class="space-y-2">
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
					use:enhance={withConfirm((fd) => ({ variant: 'info', title: t('onb.approveBtn'), message: t('onb.approveConfirm', { name: m.display_name, mdr: feeText(fd, 'mdr'), platform: feeText(fd, 'platform') }) + flatText(fd), confirmText: t('ma.approve.ok'), successMessage: t('ma.approve.done') }), busyWrap)}
					class="space-y-2"
				>
					<h4 class="font-bold">{t('onb.approveBtn')}</h4>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<FeeInput name="mdr" label={t('ma.mdr')} />
						<FeeInput name="platform" label={t('ma.fee')} />
					</div>
					<!-- Aturan transaksi kecil (BI): transaksi sampai batas tertentu dikenai MDR flat -->
					<div class="rounded-xl border border-ink/10 bg-ink/[0.03] p-3">
						<label class="flex cursor-pointer items-start gap-2">
							<input type="checkbox" name="flat_on" class="mt-0.5 shrink-0 accent-merah" bind:checked={flatOn} />
							<span class="min-w-0 flex-1 break-words">
								<b>{t('ma.flat.title')}</b>
								<span class="muted block leading-relaxed">{t('ma.flat.desc')}</span>
							</span>
						</label>
						{#if flatOn}
							<div class="mt-2 flex flex-wrap items-end gap-2">
								<label class="min-w-0 flex-1 basis-40"><span class="mb-1 block text-[11px] font-bold">{t('ma.flat.upTo')}</span>
									<input class="inp w-full py-2" name="flat_up_to" inputmode="numeric" autocomplete="off" required placeholder={t('ma.flat.ph1')} />
								</label>
								<label class="min-w-0 flex-1 basis-40"><span class="mb-1 block text-[11px] font-bold">{t('ma.flat.fee')}</span>
									<input class="inp w-full py-2" name="flat_fee" inputmode="numeric" autocomplete="off" required placeholder={t('ma.flat.ph2')} />
								</label>
							</div>
						{/if}
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
						? { variant: 'warning', title: t('ma.suspend.title'), message: t('ma.suspend.msg', { name: m.display_name }), confirmText: t('ma.suspend.ok'), successMessage: t('ma.suspend.done') }
						: { variant: 'danger', title: t('ma.reject.title'), message: t('ma.reject.msg', { name: m.display_name }), confirmText: t('ma.reject.ok'), successMessage: t('ma.reject.done') }), busyWrap)}
					class="space-y-2"
				>
					<h4 class="font-bold">{t('ma.rejectSuspend')}</h4>
					<p class="muted">{t('ma.rejectHint')}</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('onb.noteRequired')} minlength="3" maxlength="500" required />
						<button class="btn btn-danger" type="submit" disabled={busy}>{t('fin.reject')}</button>
						<button class="btn btn-warn" type="submit" formaction="?/suspend" disabled={busy}>{t('ma.suspend')}</button>
					</div>
				</form>
			{/if}

			{#if showRevoke}
				<form method="POST" action="?/revoke" use:enhance={withConfirm((fd, action) => (action.search.includes('suspend')
						? { variant: 'warning', title: t('ma.suspend.title'), message: t('ma.suspend.msg', { name: m.display_name }), confirmText: t('ma.suspend.ok'), successMessage: t('ma.suspend.done') }
						: { variant: 'danger', title: t('onb.revokeBtn'), message: t('onb.revokeConfirm', { name: m.display_name }), confirmText: t('ma.revoke.ok'), successMessage: t('ma.revoke.done') }), busyWrap)} class="space-y-2">
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
						<button class="btn btn-warn" type="submit" formaction="?/suspend" disabled={busy}>{t('ma.suspend')}</button>
					</div>
				</form>
			{/if}

			{#if showRevoke}
				<form method="POST" action="?/unlock" use:enhance={withConfirm({ variant: 'warning', title: t('ma.unlock.q'), message: t('ma.unlock.msg', { name: m.display_name }), confirmText: t('ma.unlock.ok'), successMessage: t('ma.unlock.done') }, busyWrap)} class="space-y-2">
					<h4 class="font-bold">{t('ma.unlock.title')}</h4>
					<p class="muted">{t('ma.unlock.hint')}</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('ma.unlock.ph')} minlength="3" maxlength="500" required />
						<button class="btn btn-warn" type="submit" disabled={busy}>{t('ma.unlock.btn')}</button>
					</div>
				</form>
			{/if}

			{#if showReactivate}
				<form
					method="POST"
					action="?/reactivate"
					use:enhance={withConfirm({ variant: 'info', title: t('ma.react.q'), message: t('ma.react.msg', { name: m.display_name }), confirmText: t('ma.react.ok'), successMessage: t('ma.react.done') }, busyWrap)}
					class="space-y-2"
				>
					<h4 class="font-bold">{t('ma.react.title')}</h4>
					<p class="muted">{t('ma.react.hint')}</p>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('ma.react.ph')} minlength="3" maxlength="500" required />
						<button class="btn btn-red" type="submit" disabled={busy}>{t('ma.react.title')}</button>
					</div>
				</form>
			{/if}
		</div>
	</Modal>
{/if}
