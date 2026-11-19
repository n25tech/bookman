import type { Note } from '../lib/types';
import type { Position } from '../lib/progress';

export interface ViewerApi {
  goTo(page: number, offset?: number): Promise<void>;
  next(): void;
  prev(): void;
  /** Scroll by one screen (PDF) or turn the page (comics). */
  pageDown(): void;
  pageUp(): void;
  flashNote(note: Note): void;
}

export interface NewAnchor { page: number; quote: string; anchor_start: number; anchor_end: number }
export type { Position };
