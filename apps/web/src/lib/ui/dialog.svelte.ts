// Dialog konfirmasi & toast global (pengganti window.confirm/alert). Dirender oleh DialogHost di layout root.
import type { Key } from '../i18n/messages';

export type Variant = 'info' | 'success' | 'warning' | 'danger';
/** Toast punya dua varian tambahan: abu-abu (default) dan gelap. */
export type ToastVariant = Variant | 'default' | 'dark';

export type ConfirmOptions = {
	variant?: Variant;
	title?: string;
	message: string;
	confirmText?: string;
	cancelText?: string;
};

export type Toast = { id: number; variant: ToastVariant; title?: string; message: string };

/** Judul bawaan toast/dialog berupa kunci kamus; diterjemahkan saat ditampilkan agar mengikuti bahasa aktif. */
export const TOAST_TITLE: Record<ToastVariant, Key> = {
	default: 'dlg.title.default',
	dark: 'dlg.title.default',
	info: 'toast.info',
	success: 'dlg.title.success',
	warning: 'toast.warning',
	danger: 'toast.danger'
};

export const DEFAULT_TITLE: Record<ToastVariant, Key> = {
	default: 'dlg.title.default',
	dark: 'dlg.title.default',
	info: 'dlg.title.info',
	success: 'dlg.title.success',
	warning: 'dlg.title.warning',
	danger: 'dlg.title.danger'
};

export const ui = $state<{ confirm: (ConfirmOptions & { resolve: (ok: boolean) => void }) | null; toasts: Toast[] }>({
	confirm: null,
	toasts: []
});

let seq = 0;

/** Tampilkan dialog; resolve true bila pengguna menekan tombol konfirmasi. */
export function confirmDialog(o: ConfirmOptions): Promise<boolean> {
	// Satu dialog pada satu waktu: yang lama dianggap batal.
	ui.confirm?.resolve(false);
	return new Promise<boolean>((resolve) => {
		ui.confirm = { variant: 'warning', ...o, resolve };
	});
}

export function closeConfirm(ok: boolean) {
	const c = ui.confirm;
	ui.confirm = null;
	c?.resolve(ok);
}

/** Toast singkat (hilang sendiri). */
export function notify(variant: ToastVariant, message: string, title?: string, ms = 4200) {
	const id = ++seq;
	ui.toasts = [...ui.toasts, { id, variant, title, message }].slice(-4);
	setTimeout(() => dismissToast(id), ms);
}

export function dismissToast(id: number) {
	ui.toasts = ui.toasts.filter((t) => t.id !== id);
}
