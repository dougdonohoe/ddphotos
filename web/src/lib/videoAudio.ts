import { browser } from '$app/environment';

// The viewer's mute and volume choice for lightbox videos, carried from clip to clip.
//
// Kept in localStorage so it persists across reloads and browser sessions. Like ddp_theme
// it is a preference, not site data, so it is not scoped to siteId, and clearStoredKeys
// keeps it through logout and build changes (only ?clear removes it). The module-level
// copy covers browsers where storage throws (e.g. some private browsing modes), keeping
// the choice for as long as the page lives.
export const VIDEO_AUDIO_KEY = 'ddp_video_audio';

type VideoAudio = { muted: boolean; volume: number };
const DEFAULT: VideoAudio = { muted: false, volume: 1 };

let audio: VideoAudio = browser ? load() : { ...DEFAULT };

function load(): VideoAudio {
	try {
		const parsed = JSON.parse(localStorage.getItem(VIDEO_AUDIO_KEY) ?? 'null');
		if (
			typeof parsed?.muted === 'boolean' &&
			typeof parsed?.volume === 'number' &&
			parsed.volume >= 0 &&
			parsed.volume <= 1
		) {
			return { muted: parsed.muted, volume: parsed.volume };
		}
	} catch {
		// Unparseable or storage unavailable: fall through to the default.
	}
	return { ...DEFAULT };
}

// Applies the remembered choice to a video element.
export function applyVideoAudio(video: HTMLVideoElement): void {
	video.muted = audio.muted;
	video.volume = audio.volume;
}

// Records the video's current choice, for applyVideoAudio to hand to the next clip.
export function rememberVideoAudio(video: HTMLVideoElement): void {
	audio = { muted: video.muted, volume: video.volume };
	try {
		localStorage.setItem(VIDEO_AUDIO_KEY, JSON.stringify(audio));
	} catch {
		// Storage unavailable: the module-level copy still carries it for this page.
	}
}
