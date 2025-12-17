import { browser } from '$app/environment';
import { writable } from 'svelte/store';

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'theme';
const { subscribe, set } = writable<Theme>('light');
let currentTheme: Theme = 'light';
let initialized = false;
let followSystemPreference = true;
let mediaQuery: MediaQueryList | undefined;

const themeStore = { subscribe };

const applyTheme = (nextTheme: Theme) => {
	if (!browser) return;

	const root = document.documentElement;
	root.classList.toggle('dark', nextTheme === 'dark');
	root.dataset.theme = nextTheme;
	root.style.setProperty('color-scheme', nextTheme);
};

const updateTheme = (nextTheme: Theme, persist = false) => {
	currentTheme = nextTheme;
	set(nextTheme);
	applyTheme(nextTheme);
	if (persist && browser) {
		localStorage.setItem('theme', nextTheme);
	}
};

export const initTheme = () => {
	if (!browser || initialized) return;
	initialized = true;

	mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');

	const saved = localStorage.getItem(STORAGE_KEY);
	if (saved === 'dark' || saved === 'light') {
		followSystemPreference = false;
		updateTheme(saved);
	} else {
		followSystemPreference = true;
		updateTheme(mediaQuery.matches ? 'dark' : 'light');
	}

	const handleChange = (event: MediaQueryListEvent) => {
		if (!followSystemPreference) return;
		updateTheme(event.matches ? 'dark' : 'light');
	};

	mediaQuery.addEventListener('change', handleChange);

	window.addEventListener('storage', (event) => {
		if (event.key === STORAGE_KEY && (event.newValue === 'dark' || event.newValue === 'light')) {
			followSystemPreference = false;
			updateTheme(event.newValue as Theme);
		}
	});
};

export const setTheme = (nextTheme: Theme, persist = false) => {
	if (nextTheme !== 'light' && nextTheme !== 'dark') return;
	followSystemPreference = false;
	updateTheme(nextTheme, persist);
};

export const toggleTheme = () => {
	const next = currentTheme === 'light' ? 'dark' : 'light';
	setTheme(next, true);
};

export const theme = themeStore;
