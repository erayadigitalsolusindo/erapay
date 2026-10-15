// Pembungkus use:enhance: tampilkan dialog konfirmasi dulu; bila disetujui form dikirim ulang
// (requestSubmit) dan lolos sekali tanpa bertanya lagi. Hasilnya diumumkan lewat toast.
import type { SubmitFunction } from '$app/forms';
import { confirmDialog, notify, type ConfirmOptions } from './dialog.svelte';

/** Nama aksi untuk judul toast: judul dialog tanpa tanda tanya. */
const label = (o: ConfirmOptions) => (o.title ?? 'aksi').replace(/\?+$/, '').trim();

const approved = new WeakSet<HTMLFormElement>();

export type WithConfirmOptions = ConfirmOptions & {
	/** Toast sukses setelah server menerima; tanpa ini tidak ada toast sukses. */
	successMessage?: string;
};

/**
 * @param next logika tambahan setelah disetujui (mis. status busy); boleh mengembalikan callback hasil.
 */
export function withConfirm(
	opts: WithConfirmOptions | ((fd: FormData) => WithConfirmOptions),
	next?: SubmitFunction
): SubmitFunction {
	return (input) => {
		const { formData, formElement, submitter, cancel } = input;
		const o = typeof opts === 'function' ? opts(formData) : opts;
		if (!approved.has(formElement)) {
			cancel();
			void confirmDialog(o).then((ok) => {
				if (!ok) return;
				approved.add(formElement);
				formElement.requestSubmit(submitter);
			});
			return;
		}
		approved.delete(formElement);
		const inner = next?.(input);
		return async (args) => {
			const { result, update } = args;
			if (result.type === 'failure') {
				const msg = (result.data as { message?: string } | undefined)?.message;
				notify('danger', msg ?? 'Terjadi kesalahan di server. Coba lagi sebentar lagi.', `Gagal: ${label(o)}`);
			} else if (result.type === 'success' && o.successMessage) {
				notify('success', o.successMessage, `Berhasil: ${label(o)}`);
			}
			if (typeof inner === 'function') await inner(args);
			else await update();
		};
	};
}
