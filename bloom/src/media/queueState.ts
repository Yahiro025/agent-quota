export interface QueueEntry {
	id: string;
	title: string;
	artist: string;
	duration?: string | null;
}
export interface QueueView {
	generation: string;
	available: boolean;
	source: string;
	revision: string;
	entries: QueueEntry[];
	reason?: string | null;
	total_upcoming?: number | null;
	pending?: boolean;
	needs_check?: boolean;
}
export interface QueueState {
	generation: string;
	loading: boolean;
	view: QueueView | null;
	pending: string | null;
	message: string;
	needsCheck: boolean;
}
type Call = <T>(command: string, args: Record<string, unknown>) => Promise<T>;
const PLAYING = "Playing selected item.";

// The controller outlives the drawer's visibility. Hiding it never replays an
// action or pretends an already-dispatched source command was canceled.
export class QueueController {
	state: QueueState = {
		generation: "",
		loading: false,
		view: null,
		pending: null,
		message: "",
		needsCheck: false
	};
	private epoch = 0;
	private load = 0;
	private noteTimer: ReturnType<typeof setTimeout> | undefined;
	private listeners = new Set<() => void>();
	constructor(private call: Call) {}
	subscribe = (listener: () => void) => {
		this.listeners.add(listener);
		return () => {
			this.listeners.delete(listener);
		};
	};
	getSnapshot = () => this.state;
	private update(patch: Partial<QueueState>) {
		this.state = { ...this.state, ...patch };
		this.listeners.forEach((fn) => fn());
	}
	target(generation: string) {
		if (generation === this.state.generation) return;
		this.epoch++;
		this.load++;
		clearTimeout(this.noteTimer);
		this.update({
			generation,
			// A targeted queue is loading until its first reply, so the drawer never
			// flashes "unavailable" for a player that has a queue.
			loading: !!generation,
			view: null,
			pending: null,
			message: "",
			needsCheck: false
		});
	}
	async refresh(manual = false) {
		const { generation } = this.state,
			epoch = this.epoch,
			load = ++this.load;
		if (!generation) return;
		this.update({ loading: !this.state.view });
		try {
			const view = await this.call<QueueView>("media_queue_get", { generation });
			if (epoch !== this.epoch || load !== this.load) return;
			if (view.generation !== generation) throw new Error("stale");
			this.update({
				view,
				loading: false,
				...(manual && !view.pending && !view.needs_check ? { needsCheck: false, message: "" } : {}),
				...(view.needs_check ? { needsCheck: true } : {})
			});
		} catch {
			if (epoch === this.epoch && load === this.load)
				this.update({
					loading: false,
					view: null,
					message: "Player disconnected or changed. Check the queue again.",
					needsCheck: true
				});
		}
	}
	async select(id: string) {
		const { generation, view, pending, needsCheck } = this.state;
		if (
			pending ||
			needsCheck ||
			view?.pending ||
			view?.needs_check ||
			!view?.available ||
			!view.entries.some((e) => e.id === id)
		)
			return;
		const epoch = this.epoch;
		this.update({ pending: id, message: "" });
		let timer: ReturnType<typeof setTimeout> | undefined;
		try {
			// IPC failure must not leave the UI spinning forever. The backend owns
			// the five-second playback deadline; this only bounds a lost IPC reply.
			const result = await Promise.race([
				this.call<string>("media_queue_select", {
					generation,
					revision: view.revision,
					entryId: id
				}),
				new Promise<string>((resolve) => {
					timer = setTimeout(() => resolve("uncertain"), 5500);
				})
			]);
			if (epoch !== this.epoch) return;
			if (result === "confirmed") {
				this.update({ pending: null, message: PLAYING });
				// A success note goes stale once the next item is chosen or plays on.
				// Each note gets its own full time, so an earlier timer must not clear it.
				clearTimeout(this.noteTimer);
				this.noteTimer = setTimeout(() => {
					if (epoch === this.epoch && this.state.message === PLAYING) this.update({ message: "" });
				}, 4000);
			}
			else if (result === "stale")
				this.update({ pending: null, message: "Queue changed. Choose from the updated list." });
			else
				this.update({
					pending: null,
					needsCheck: true,
					message: "Playback is uncertain. Check playback and the queue before trying again."
				});
			await this.refresh();
		} catch {
			if (epoch === this.epoch)
				this.update({
					pending: null,
					needsCheck: true,
					message: "Could not confirm playback. Check playback and the queue before trying again."
				});
		} finally {
			if (timer !== undefined) clearTimeout(timer);
		}
	}
}
