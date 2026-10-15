<script lang="ts">
	// Tombol "Aksi" + popup berisi semua tindakan review untuk satu merchant (sesuai status & izin).
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '$app/forms';
	import Modal from './Modal.svelte';
	import Combobox from './Combobox.svelte';
	import { getI18n } from '../i18n/i18n.svelte';

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
	const showVerify = $derived(canApprove && !m.owner_email_verified);
	const showStart = $derived(canReview && m.status === 'submitted');
	const showChanges = $derived(canReview && ['submitted', 'under_review'].includes(m.status));
	const showDecide = $derived(canApprove && reviewable.includes(m.status));
	const showRevoke = $derived(canApprove && m.status === 'approved');
	const any = $derived(showVerify || showStart || showChanges || showDecide || showRevoke);

	const pct = (bps: FormDataEntryValue | null) => (Number(bps) / 100).toString();
	const submit =
		(confirmMsg?: (fd: FormData) => string): SubmitFunction =>
		({ formData, cancel }) => {
			if (confirmMsg && !confirm(confirmMsg(formData))) return cancel();
			busy = true;
			err = '';
			return async ({ result, update }) => {
				busy = false;
				if (result.type === 'failure') {
					err = (result.data as { message?: string })?.message ?? t('c.loadFail');
					return;
				}
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
				<form method="POST" action="?/verifyEmail" use:enhance={submit(() => 'Tandai email pemilik sebagai terverifikasi secara manual?')} class="space-y-2">
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
				<form method="POST" action="?/startReview" use:enhance={submit()} class="flex items-center justify-between gap-3">
					<div><h4 class="font-bold">{t('onb.startReview')}</h4><p class="muted">Tandai bahwa kamu sedang meninjau data ini.</p></div>
					<input type="hidden" name="id" value={m.id} />
					<button class="btn" type="submit" disabled={busy}>{t('onb.startReview')}</button>
				</form>
			{/if}

			{#if showChanges}
				<form method="POST" action="?/requestChanges" use:enhance={submit()} class="space-y-2">
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
					use:enhance={submit((fd) => t('onb.approveConfirm', { name: m.display_name, mdr: pct(fd.get('mdr_bps')), platform: pct(fd.get('platform_bps')) }))}
					class="space-y-2"
				>
					<h4 class="font-bold">{t('onb.approveBtn')}</h4>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp w-36 py-2" name="mdr_bps" type="number" min="0" max="10000" step="1" inputmode="numeric" required placeholder={t('onb.mdr')} aria-label={t('onb.mdr')} />
						<input class="inp w-48 py-2" name="platform_bps" type="number" min="0" max="10000" step="1" inputmode="numeric" required placeholder={t('onb.platform')} aria-label={t('onb.platform')} />
						<button class="btn btn-red" type="submit" disabled={busy}>{t('onb.approveBtn')}</button>
					</div>
					<p class="muted">{t('onb.bpsHint')}</p>
				</form>
				<form method="POST" action="?/reject" use:enhance={submit()} class="space-y-2">
					<h4 class="font-bold">{t('fin.reject')}</h4>
					<input type="hidden" name="id" value={m.id} />
					<div class="flex flex-wrap gap-2">
						<input class="inp min-w-0 flex-1 py-2" name="note" placeholder={t('onb.noteRequired')} minlength="3" maxlength="500" required />
						<button class="btn btn-danger" type="submit" disabled={busy}>{t('fin.reject')}</button>
					</div>
				</form>
			{/if}

			{#if showRevoke}
				<form method="POST" action="?/revoke" use:enhance={submit(() => t('onb.revokeConfirm', { name: m.display_name }))} class="space-y-2">
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
					</div>
				</form>
			{/if}
		</div>
	</Modal>
{/if}
