package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
)

// Sort modes of a folder. Folders always come first; "name" is the default
// and is not stored.
const (
	SortName     = "name"
	SortModified = "modified"
	SortCreated  = "created"
	SortManual   = "manual"
)

// SortModes are the modes a folder can have.
var SortModes = []string{SortName, SortModified, SortCreated, SortManual}

// MaxOrderNames is how many names a manual order keeps.
const MaxOrderNames = 10000

// FolderOrder is how a folder's entries are sorted; in manual mode Names
// lists them in order (entries not listed follow by name).
type FolderOrder struct {
	Mode  string   `json:"mode"`
	Names []string `json:"names,omitempty"`
}

// FolderOrders returns the sort orders of a soma's folders by folder path.
func (s *Store) FolderOrders(ctx context.Context, somaID string) (map[string]FolderOrder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT folder, mode, names FROM folder_order WHERE soma_id = ?`, somaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FolderOrder{}
	for rows.Next() {
		var folder, mode, names string
		if err := rows.Scan(&folder, &mode, &names); err != nil {
			return nil, err
		}
		o := FolderOrder{Mode: mode}
		if err := json.Unmarshal([]byte(names), &o.Names); err != nil {
			o.Names = nil
		}
		out[folder] = o
	}
	return out, rows.Err()
}

// SetFolderOrder stores how folder is sorted. Sorting by name is the default
// and removes the entry; names count only in manual mode.
func (s *Store) SetFolderOrder(ctx context.Context, somaID, folder string, o FolderOrder) error {
	if !slices.Contains(SortModes, o.Mode) {
		return fmt.Errorf("unknown sort mode %q", o.Mode)
	}
	if o.Mode == SortName {
		_, err := s.db.ExecContext(ctx, `DELETE FROM folder_order WHERE soma_id = ? AND folder = ?`, somaID, folder)
		return err
	}
	names := o.Names
	if o.Mode != SortManual || names == nil {
		names = []string{}
	}
	if len(names) > MaxOrderNames {
		names = names[:MaxOrderNames]
	}
	data, err := json.Marshal(names)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO folder_order (soma_id, folder, mode, names) VALUES (?, ?, ?, ?)
		ON CONFLICT (soma_id, folder) DO UPDATE SET mode = excluded.mode, names = excluded.names`,
		somaID, folder, o.Mode, string(data))
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
