/** A button in a Monitoring area's SectionBar. */
export interface Section<K extends string> {
	key: K;
	label: string;
	/** Shown next to the label when above zero. */
	count?: number;
}
