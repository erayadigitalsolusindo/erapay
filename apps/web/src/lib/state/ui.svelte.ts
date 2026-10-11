// Reactive state (Svelte 5 runes). Contoh: sidebar terbuka/tutup.
class UiState {
	sidebarOpen = $state(true);
	toggleSidebar() {
		this.sidebarOpen = !this.sidebarOpen;
	}
}
export const ui = new UiState();
