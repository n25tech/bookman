-- Notes written while the server was unreachable are queued in the browser and
-- sent later. The client picks a random client_id for each new note so that a
-- create that is retried (because the answer was lost) doesn't make a copy.
ALTER TABLE notes ADD COLUMN client_id TEXT;
CREATE UNIQUE INDEX notes_user_client_id_idx ON notes(user_id, client_id) WHERE client_id IS NOT NULL;
