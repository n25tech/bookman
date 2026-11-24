// The browser database shared by the offline queues (progress and notes).
const DB_NAME = 'bookman';
// Queued progress, one entry per user and book (key "<user id>:<book id>").
export const PROGRESS_STORE = 'pending-progress-v2';
// Version 1–2 store, keyed by book only; its entries move to PROGRESS_STORE.
const OLD_PROGRESS_STORE = 'pending-progress';
export const NOTES_STORE = 'pending-notes';

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 3);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(NOTES_STORE)) db.createObjectStore(NOTES_STORE, { keyPath: 'key' });
      if (!db.objectStoreNames.contains(PROGRESS_STORE)) {
        const store = db.createObjectStore(PROGRESS_STORE, { keyPath: 'key' });
        // Entries queued before progress was tied to a user: nobody knows whose
        // they are, so they keep the old behaviour (sent by whoever signs in).
        if (db.objectStoreNames.contains(OLD_PROGRESS_STORE)) {
          const old = req.transaction!.objectStore(OLD_PROGRESS_STORE).getAll();
          old.onsuccess = () => {
            for (const p of old.result) store.put({ ...p, key: `legacy:${p.bookId}`, userId: null });
            db.deleteObjectStore(OLD_PROGRESS_STORE);
          };
        }
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
    // Another tab still has an older version open; it closes it on versionchange below.
    req.onblocked = () => console.warn('bookman database upgrade waiting for other tabs');
  });
}

let db: Promise<IDBDatabase> | null = null;
function get(): Promise<IDBDatabase> {
  if (!db) {
    db = openDB().then((d) => {
      // Let a newer version in another tab upgrade the database.
      d.onversionchange = () => { d.close(); db = null; };
      return d;
    }, (e) => { db = null; throw e; });
  }
  return db;
}

export async function idb<T>(store: string, mode: IDBTransactionMode, f: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const d = await get();
  return new Promise((resolve, reject) => {
    const tx = d.transaction(store, mode);
    const req = f(tx.objectStore(store));
    tx.oncomplete = () => resolve(req.result);
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}
