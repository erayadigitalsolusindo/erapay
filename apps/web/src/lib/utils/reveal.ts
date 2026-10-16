// Action Svelte: elemen muncul (fade + naik) saat masuk viewport. Hormati prefers-reduced-motion.
export function reveal(node: HTMLElement) {
	if (typeof IntersectionObserver === 'undefined' || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
	node.style.opacity = '0';
	node.style.transform = 'translateY(26px)';
	node.style.transition = 'opacity .7s cubic-bezier(.2,.7,.2,1), transform .7s cubic-bezier(.2,.7,.2,1)';
	const io = new IntersectionObserver(
		(es) => {
			for (const e of es)
				if (e.isIntersecting) {
					node.style.opacity = '1';
					node.style.transform = 'none';
					io.disconnect();
				}
		},
		{ threshold: 0.12 }
	);
	io.observe(node);
	return { destroy: () => io.disconnect() };
}
