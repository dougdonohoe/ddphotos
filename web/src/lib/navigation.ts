export type Rect = { left: number; top: number; width: number; height: number };
export type Direction = 'left' | 'right' | 'up' | 'down';

// Returns the index to focus given a list of rects and a direction, or null if at the boundary.
// Rects can come from justified-layout boxes or getBoundingClientRect() — both expose the same
// {left, top, width, height} shape. A 10px tolerance on row matching handles sub-pixel rounding
// from CSS grid layouts (justified-layout emits exact values so tolerance has no effect there).
export function navigateCursor(
	rects: Rect[],
	currentIndex: number,
	direction: Direction
): number | null {
	const count = rects.length;
	if (count === 0) return null;

	if (direction === 'left') return currentIndex > 0 ? currentIndex - 1 : null;
	if (direction === 'right') return currentIndex < count - 1 ? currentIndex + 1 : null;

	const current = rects[currentIndex];
	const currentCenterX = current.left + current.width / 2;
	const isUp = direction === 'up';

	const candidates = rects
		.map((rect, i) => ({ rect, i }))
		.filter(({ rect }) => (isUp ? rect.top < current.top : rect.top > current.top));
	if (candidates.length === 0) return null;

	const nearestRowTop = isUp
		? Math.max(...candidates.map(({ rect }) => rect.top))
		: Math.min(...candidates.map(({ rect }) => rect.top));
	const rowItems = candidates.filter(({ rect }) => Math.abs(rect.top - nearestRowTop) <= 10);

	return rowItems.reduce((bestIdx, { rect, i }) => {
		const dist = Math.abs(rect.left + rect.width / 2 - currentCenterX);
		const bestDist = Math.abs(rects[bestIdx].left + rects[bestIdx].width / 2 - currentCenterX);
		return dist < bestDist ? i : bestIdx;
	}, rowItems[0].i);
}
