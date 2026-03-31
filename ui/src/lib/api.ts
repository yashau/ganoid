const BASE = '/api';
const TOKEN_KEY = 'ganoid_token';

/** Read token from ?token= on first load, then always from localStorage. */
function getToken(): string {
	if (typeof window === 'undefined') return '';
	const params = new URLSearchParams(window.location.search);
	const urlToken = params.get('token');
	if (urlToken) {
		localStorage.setItem(TOKEN_KEY, urlToken);
		// Clean the token out of the URL without a page reload
		params.delete('token');
		const newSearch = params.toString();
		const newUrl = window.location.pathname + (newSearch ? '?' + newSearch : '') + window.location.hash;
		history.replaceState(null, '', newUrl);
		return urlToken;
	}
	return localStorage.getItem(TOKEN_KEY) ?? '';
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(BASE + path, {
		method,
		headers: {
			...(body ? { 'Content-Type': 'application/json' } : {}),
			Authorization: `Bearer ${getToken()}`
		},
		body: body ? JSON.stringify(body) : undefined
	});
	if (!res.ok) {
		const err = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(err.error ?? res.statusText);
	}
	if (res.status === 204) return undefined as T;
	return res.json();
}

export interface Profile {
	id: string;
	name: string;
	login_server: string;
	created_at: string;
	last_used: string;
}

export interface ProfileStore {
	active_profile_id: string;
	profiles: Profile[];
}

export interface TailscaleInfo {
	backend_state: string;
	peer_count: number;
}

export interface StatusResponse {
	active_profile: Profile;
	version: string;
	tailscale: TailscaleInfo;
	tailscale_error?: string;
}

export interface SwitchEvent {
	step: number;
	total: number;
	message: string;
	error?: string;
	done: boolean;
}

export const api = {
	status: () => req<StatusResponse>('GET', '/status'),

	profiles: () => req<ProfileStore>('GET', '/profiles'),

	createProfile: (id: string, name: string, login_server: string) =>
		req<Profile>('POST', '/profiles', { id, name, login_server }),

	updateProfile: (id: string, name: string, login_server: string) =>
		req<Profile>('PUT', `/profiles/${id}`, { name, login_server }),

	deleteProfile: (id: string) => req<void>('DELETE', `/profiles/${id}`),

	/** Streams switch progress events via SSE. Returns a cleanup function. */
	switchProfile(
		id: string,
		onEvent: (e: SwitchEvent) => void,
		onDone: () => void,
		onError: (msg: string) => void
	): () => void {
		const controller = new AbortController();

		fetch(`${BASE}/profiles/${id}/switch`, {
			method: 'POST',
			headers: { Authorization: `Bearer ${getToken()}` },
			signal: controller.signal
		}).then(async (res) => {
			if (!res.ok || !res.body) {
				onError(`Switch failed: ${res.statusText}`);
				return;
			}
			const reader = res.body.getReader();
			const decoder = new TextDecoder();
			let buf = '';

			while (true) {
				const { value, done } = await reader.read();
				if (done) break;
				buf += decoder.decode(value, { stream: true });
				const lines = buf.split('\n');
				buf = lines.pop() ?? '';
				for (const line of lines) {
					if (line.startsWith('data: ')) {
						try {
							const ev: SwitchEvent = JSON.parse(line.slice(6));
							onEvent(ev);
							if (ev.done) {
								if (ev.error) onError(ev.error);
								else onDone();
								return;
							}
						} catch {
							// ignore malformed SSE line
						}
					}
				}
			}
		}).catch((err) => {
			if (err.name !== 'AbortError') onError(String(err));
		});

		return () => controller.abort();
	},

	tailscaleStatus: () => req<unknown>('GET', '/tailscale/status')
};
