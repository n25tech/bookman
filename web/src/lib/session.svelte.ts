import type { User } from './types';

export const session = $state<{ user: User | null; setupRequired: boolean; ready: boolean }>({
  user: null,
  setupRequired: false,
  ready: false,
});
