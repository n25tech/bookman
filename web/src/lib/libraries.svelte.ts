// The library list, shared by the header, home page and admin page, so a
// library added or renamed in Admin shows up everywhere at once.
import { api } from './api';
import type { Library } from './types';

export const libraries = $state<{ list: Library[]; loaded: boolean }>({ list: [], loaded: false });

export async function refreshLibraries(): Promise<Library[]> {
  try {
    libraries.list = await api.libraries();
  } finally {
    libraries.loaded = true;
  }
  return libraries.list;
}
