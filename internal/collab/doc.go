package collab

import (
	"errors"
	"strings"
	"sync"
	"time"

	"dendronotes/internal/events"
	"dendronotes/internal/soma"
)

// doc is one live document. All fields are guarded by mu.
type doc struct {
	m    *Manager
	mu   sync.Mutex
	path string
	id   string // instance: a new one (after closing) means clients rejoin

	loaded, closed bool
	readOnly       bool

	text    []uint16
	version int
	first   int // version before updates[0]
	updates []Update
	kept    int // bytes of changes in updates

	format format
	saved  string // text as last read from or written to disk (merge base)
	etag   string // version of the file that saved corresponds to
	// savedVersion is the version written to disk.
	savedVersion int
	dirty        bool
	dirtySince   time.Time
	timer        *time.Timer
	idle         *time.Timer // closes the document once nobody is in it

	peers map[string]*peer // by stream id
}

type peer struct {
	id    string // public, shown to others
	user  User
	color int
	sel   *Selection
}

// load reads the note from disk.
func (d *doc) load() error {
	n, err := d.m.opts.Soma.ReadNote(d.path)
	if err != nil {
		return err
	}
	text, f := normalize(n.Content)
	d.id = randomID(9)
	d.text = fromString(text)
	d.format = f
	d.saved = text
	d.etag = n.ETag
	d.readOnly = n.ReadOnly
	d.loaded = true
	return nil
}

func (d *doc) join(st *stream, rec *Recover) Joined {
	res := Joined{}
	if rec != nil && rec.Doc != d.id && rec.Mine != rec.Base && !d.readOnly {
		current := toString(d.text)
		merged, conflict := merge3(normalizeOnly(rec.Base), normalizeOnly(rec.Mine), current)
		if merged != current && int64(len(merged)) <= d.m.opts.MaxBytes {
			d.serverUpdate(fromString(merged))
		}
		res.Conflict = conflict
	}
	if d.idle != nil {
		d.idle.Stop()
		d.idle = nil
	}
	p := &peer{id: randomID(6), user: st.user, color: int(st.user.ID % colors)}
	if old := d.peers[st.id]; old != nil {
		p.id = old.id // joining again from the same tab
	}
	d.peers[st.id] = p
	d.sendPeers()
	res.Doc, res.Version, res.Text = d.id, d.version, toString(d.text)
	res.ReadOnly, res.Saved, res.Peer, res.Peers = d.readOnly, d.savedVersion, p.id, d.peerList()
	return res
}

// commit records updates that turned the document into text and sends them
// to everyone in the note (the authors confirm their own updates this way).
func (d *doc) commit(text []uint16, updates []Update) {
	from := d.version
	d.text = text
	d.updates = append(d.updates, updates...)
	d.version += len(updates)
	for _, u := range updates {
		d.kept += len(u.Changes)
	}
	for len(d.updates) > keepUpdates || (d.kept > keepBytes && len(d.updates) > 1) {
		d.kept -= len(d.updates[0].Changes)
		d.updates = d.updates[1:]
		d.first++
	}
	d.send(events.Event{Name: "collab", Data: map[string]any{
		"path": d.path, "doc": d.id, "from": from, "updates": updates,
	}})
	d.markDirty()
}

// serverUpdate replaces the document with text as an update of the server
// (merges of changes made elsewhere).
func (d *doc) serverUpdate(text []uint16) {
	d.commit(text, []Update{{ClientID: "", Changes: diffJSON(toString(d.text), toString(text))}})
}

func (d *doc) markDirty() {
	now := time.Now()
	if !d.dirty {
		d.dirty = true
		d.dirtySince = now
	}
	delay := min(d.m.opts.SaveDelay, max(d.dirtySince.Add(d.m.opts.MaxDelay).Sub(now), 0))
	if d.timer == nil {
		d.timer = time.AfterFunc(delay, d.saveLater)
	} else {
		d.timer.Reset(delay)
	}
}

func (d *doc) saveLater() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.save()
	}
}

func (d *doc) stop() {
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	if d.idle != nil {
		d.idle.Stop()
		d.idle = nil
	}
}

// save writes the document to its file if it has unsaved changes. A file
// changed elsewhere in the meantime is merged first, never overwritten.
func (d *doc) save() {
	if !d.dirty || !d.loaded || d.readOnly {
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		text := toString(d.text)
		n, err := d.m.opts.Soma.WriteNote(d.path, d.format.apply(text), d.etag)
		var ce *soma.ConflictError
		switch {
		case err == nil:
			d.saved, d.etag, d.savedVersion = text, n.ETag, d.version
			d.dirty = false
			d.stop()
			d.send(events.Event{Name: "saved", Data: map[string]any{"path": d.path, "doc": d.id, "version": d.version}})
			if d.m.opts.Saved != nil {
				d.m.opts.Saved(n)
			}
			return
		case errors.As(err, &ce):
			if gone := d.checkDisk(); gone {
				return
			}
			if !d.dirty {
				return // the merge produced exactly the file
			}
		case errors.Is(err, soma.ErrNotFound):
			d.removedElsewhere()
			return
		case errors.Is(err, soma.ErrReadOnly):
			d.readOnly = true
			d.notice("read_only", nil)
			return
		default:
			d.m.opts.Log.Error("saving note failed", "path", d.path, "err", err)
			d.notice("save_failed", nil)
			d.timer = time.AfterFunc(d.m.opts.MaxDelay, d.saveLater)
			return
		}
	}
}

// checkDisk compares the file with what was last saved or read and merges
// changes made elsewhere into the document. It reports whether the note is
// gone (the document is then closed).
func (d *doc) checkDisk() bool {
	if d.closed || !d.loaded {
		return false
	}
	n, err := d.m.opts.Soma.ReadNote(d.path)
	if errors.Is(err, soma.ErrNotFound) {
		d.removedElsewhere()
		return true
	}
	if err != nil {
		d.m.opts.Log.Warn("checking open note failed", "path", d.path, "err", err)
		return false
	}
	if n.ETag == d.etag {
		return false
	}
	theirs, f := normalize(n.Content)
	ours := toString(d.text)
	merged, conflict := merge3(d.saved, ours, theirs)
	edited := ours != d.saved
	d.saved, d.etag, d.format, d.readOnly = theirs, n.ETag, f, n.ReadOnly
	if merged != ours {
		d.serverUpdate(fromString(merged))
	}
	if merged == theirs {
		d.dirty = false
		d.savedVersion = d.version
		d.stop()
		d.send(events.Event{Name: "saved", Data: map[string]any{"path": d.path, "doc": d.id, "version": d.version}})
	} else {
		d.markDirty()
	}
	if edited {
		kind := "merged"
		if conflict {
			kind = "conflict"
		}
		d.notice(kind, nil)
	}
	return false
}

func (d *doc) removedElsewhere() {
	d.notice("removed", nil)
	d.stop()
	d.closed = true
}

// notice tells everyone in the note about something that happened to it.
func (d *doc) notice(kind string, extra map[string]any) {
	data := map[string]any{"path": d.path, "doc": d.id, "kind": kind}
	for k, v := range extra {
		data[k] = v
	}
	d.send(events.Event{Name: "notice", Data: data})
}

func (d *doc) peerList() []Peer {
	out := make([]Peer, 0, len(d.peers))
	for _, p := range d.peers {
		out = append(out, Peer{ID: p.id, Name: p.user.Name, Color: p.color, Sel: p.sel})
	}
	return out
}

func (d *doc) sendPeers() {
	d.send(events.Event{Name: "presence", Data: map[string]any{"path": d.path, "doc": d.id, "peers": d.peerList()}})
}

func (d *doc) send(e events.Event) {
	if d.m.opts.Send == nil || len(d.peers) == 0 {
		return
	}
	ids := make([]string, 0, len(d.peers))
	for id := range d.peers {
		ids = append(ids, id)
	}
	d.m.opts.Send(ids, e)
}

// format is how a file writes what the editor shows as plain lines.
type format struct {
	bom  bool
	crlf bool
}

// normalize turns file content into the editor's text: without a byte order
// mark and with "\n" line breaks (CodeMirror splits at \r\n, \r and \n).
func normalize(content string) (string, format) {
	var f format
	if strings.HasPrefix(content, "\ufeff") {
		f.bom = true
		content = content[len("\ufeff"):]
	}
	crlf := strings.Count(content, "\r\n")
	f.crlf = crlf > 0 && crlf >= strings.Count(content, "\n")-crlf
	return normalizeOnly(content), f
}

func normalizeOnly(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// apply turns editor text back into file content.
func (f format) apply(text string) []byte {
	if f.crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if f.bom {
		text = "\ufeff" + text
	}
	return []byte(text)
}
