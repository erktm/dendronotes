package store

import (
	"context"
	"encoding/json"
	"path"
	"slices"
)

// MaxOrderNames is how many names a manual order keeps.
const MaxOrderNames = 10000

// FolderOrders returns the orders people made by hand, by folder (” is the
// top level): the names of a folder's entries in order. How the tree is
// sorted (by name, date or this order) each person chooses in the app.
func (s *Store) FolderOrders(ctx context.Context, somaID string) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT folder, names FROM folder_order WHERE soma_id = ?`, somaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var folder, raw string
		if err := rows.Scan(&folder, &raw); err != nil {
			return nil, err
		}
		var names []string
		if json.Unmarshal([]byte(raw), &names) == nil && len(names) > 0 {
			out[folder] = names
		}
	}
	return out, rows.Err()
}

// SetFolderOrder stores the order of a folder made by hand; no names removes it.
func (s *Store) SetFolderOrder(ctx context.Context, somaID, folder string, names []string) error {
	if len(names) == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM folder_order WHERE soma_id = ? AND folder = ?`, somaID, folder)
		return err
	}
	if len(names) > MaxOrderNames {
		names = names[:MaxOrderNames]
	}
	data, err := json.Marshal(names)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO folder_order (soma_id, folder, mode, names) VALUES (?, ?, 'manual', ?)
		ON CONFLICT (soma_id, folder) DO UPDATE SET mode = 'manual', names = excluded.names`,
		somaID, folder, string(data))
	return err
}

const underFolder = `(folder = ?2 OR substr(folder, 1, length(?2) + 1) = ?2 || '/')`

// MoveFolderOrder lets the sort orders follow an entry from from to to: the
// orders of a moved folder and everything below it go along, and a renamed
// entry keeps its place in its folder's manual order.
func (s *Store) MoveFolderOrder(ctx context.Context, somaID, from, to string) error {
	if from == to || from == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE OR REPLACE folder_order SET folder = ?3 || substr(folder, length(?2) + 1)
		WHERE soma_id = ?1 AND `+underFolder, somaID, from, to); err != nil {
		return err
	}
	if parent := dir(from); parent == dir(to) {
		var raw string
		err := tx.QueryRowContext(ctx, `SELECT names FROM folder_order WHERE soma_id = ? AND folder = ?`, somaID, parent).Scan(&raw)
		if err == nil {
			var names []string
			if json.Unmarshal([]byte(raw), &names) == nil {
				if i := slices.Index(names, path.Base(from)); i >= 0 {
					names[i] = path.Base(to)
					data, _ := json.Marshal(names)
					if _, err := tx.ExecContext(ctx, `UPDATE folder_order SET names = ? WHERE soma_id = ? AND folder = ?`,
						string(data), somaID, parent); err != nil {
						return err
					}
				}
			}
		}
	}
	return tx.Commit()
}

// RemoveFolderOrder deletes the sort orders of p and the folders below it;
// an empty path means the whole soma.
func (s *Store) RemoveFolderOrder(ctx context.Context, somaID, p string) error {
	cond := `soma_id = ?1 AND ` + underFolder
	if p == "" {
		cond = `soma_id = ?1 AND ?2 = ''`
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM folder_order WHERE `+cond, somaID, p)
	return err
}

// dir is the folder of a soma path ("" at the top level).
func dir(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}
