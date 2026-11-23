package store

import (
	"context"
	"testing"
)

func TestIconMono(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	dark, err := s.AddIcon(ctx, ServerIcons, "Dark", "image/svg+xml", []byte("<svg/>"), true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !dark.Mono {
		t.Errorf("stored as monochrome: %+v", dark)
	}
	// Icons stored before the column existed have no answer yet.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO soma_icons (soma_id, name, mime, data, created_at) VALUES ('', 'Old', 'image/svg+xml', 'old', 0), ('', 'Older', 'image/svg+xml', 'older', 0)`); err != nil {
		t.Fatal(err)
	}
	asked := 0
	mono := func(mime string, data []byte) bool {
		asked++
		return string(data) == "old"
	}
	if err := s.FillIconMono(ctx, ServerIcons, mono); err != nil {
		t.Fatal(err)
	}
	if err := s.FillIconMono(ctx, ServerIcons, mono); err != nil {
		t.Fatal(err)
	}
	if asked != 2 {
		t.Errorf("each unknown icon is looked at once, not %d times", asked)
	}
	icons, err := s.Icons(ctx, ServerIcons)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, i := range icons {
		got[i.Name] = i.Mono
	}
	if !got["Dark"] || !got["Old"] || got["Older"] {
		t.Errorf("mono after filling: %v", got)
	}
}
