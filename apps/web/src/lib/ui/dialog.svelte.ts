// Dialog konfirmasi & toast global (pengganti window.confirm/alert). Dirender oleh DialogHost di layout root.
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

export type Toast = { id: number; variant: ToastVariant; title: string; message: string };

const TOAST_TITLE: Record<ToastVariant, string> = {
	default: 'Pemberitahuan',
	dark: 'Pemberitahuan',
	info: 'Info',
	success: 'Berhasil',
	warning: 'Perhatian',
	danger: 'Gagal'
};

export const DEFAULT_TITLE: Record<ToastVariant, string> = {
	default: 'Pemberitahuan',
	dark: 'Pemberitahuan',
	info: 'Informasi',
	success: 'Berhasil',
	warning: 'Konfirmasi dulu ya',
	danger: 'Yakin melanjutkan?'
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
	ui.toasts = [...ui.toasts, { id, variant, title: title ?? TOAST_TITLE[variant], message }].slice(-4);
	setTimeout(() => dismissToast(id), ms);
}

export function dismissToast(id: number) {
	ui.toasts = ui.toasts.filter((t) => t.id !== id);
}
