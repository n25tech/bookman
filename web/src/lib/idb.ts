// The browser database shared by the offline queues (progress and notes).
const DB_NAME = 'bookman';
export const PROGRESS_STORE = 'pending-progress';
export const NOTES_STORE = 'pending-notes';

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 2);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(PROGRESS_STORE)) db.createObjectStore(PROGRESS_STORE, { keyPath: 'bookId' });
      if (!db.objectStoreNames.contains(NOTES_STORE)) db.createObjectStore(NOTES_STORE, { keyPath: 'key' });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
    // Another tab still has version 1 open; it closes it on versionchange below.
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
