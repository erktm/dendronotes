# User guide

How to work with dendronotes: notes, synapses, drawings, sharing and the administration. For
installing and running it see [installation](installation.md); for signing in with Keycloak or
Active Directory see [authentication](authentication.md).

## Vocabulary

| Term        | Meaning                                                                          |
|-------------|----------------------------------------------------------------------------------|
| **soma**    | A folder that holds notes, the cell body everything grows from. Plural: somas. Every account has its own. |
| **note**    | A Markdown file in the soma.                                                     |
| **synapse** | A connection between two notes, e.g. the wiki link `[[Note]]`. Plural: synapses. |
| backlinks   | The synapses pointing *to* a note ("Incoming synapses").                         |

Files in a soma remain plain, Obsidian-compatible Markdown; the wiki link syntax is the usual
`[[Note]]`, `[[Note|Alias]]` and `[[Note#Heading]]`. A folder you use with Obsidian can be opened as a soma.

## Writing notes

- **Edit and Read.** The editor renders Markdown as you type (live preview); the line with
  the cursor shows its syntax. The reading view shows the finished note. Switch with the
  toggle in the top bar or ⌘E / Ctrl+E.
- **Synapses.** Type `[[` for suggestions from the soma, and `[[Note#` for the headings of
  that note (`[[#` for this note's own). Click a synapse to open the note;
  in the editor, on the line you are editing, hold ⌘ / Ctrl while clicking. A synapse whose
  note does not exist yet is shown dashed; clicking it creates the note next to the current
  one (or at the path given, e.g. `[[Projects/Idea]]`). Markdown links to notes, such as
  `[Text](Other.md)`, are synapses too.
- **Tasks.** Checkboxes (`- [ ]`) can be ticked in both views; the file changes accordingly.
- **Details panel** (on wide screens). Outline of the note, its synapses, and the
  **incoming synapses**: every note that connects here, with the line it does so in.
- **Search** (⌘⇧F / Ctrl+Shift+F, or the magnifier in the left rail). Words match their
  beginning, `"quoted text"` an exact phrase and `#tag` a tag, including nested tags such as
  `#garden/beds`; case and accents are ignored. A result opens the note at the matching line.
  With an empty search field, the panel lists all tags.
- **Tags.** `#tag` anywhere in the text (not in code) or a `tags:` list in the front matter.
- **Attachments.** Paste an image or drop files into the editor: they are uploaded next to the
  note and embedded as `![[photo.png]]`. Files dropped on a folder in the tree (or picked with
  the upload button) go into that folder. Embeds show images (`![[photo.png|300]]` sets the
  width), audio and video players; other files, such as PDFs, become links that open them.
  Standard Markdown works too: `![alt](photo.png)`, `[report](report.pdf)`. Clicking a file in
  the tree shows it with its size and a download button. Images from other hosts are never
  loaded.
- **Drawings.** Simple diagrams and sketches: "New drawing" behind the arrow next to the `+` at
  the top of the file tree (or in the tree's context menu), or the drawing button in the top bar
  while editing, which puts a new drawing next to the note, embeds it at the cursor
  (`![[Drawing 2026-11-10 1530.drawing.svg]]`) and opens it. To show an existing drawing in a
  note, type `![[` and pick it from the suggestions (they list drawings and images), or copy the
  line from "Embed in a note" in the drawing editor. The editor has
  rectangles, ellipses, diamonds, lines and arrows, a pen, text, and icons; double-click a shape
  to write in it. The ends of lines and arrows stick to shapes, text and icons and follow them
  when they move; arrows can bend at right angles. Colors come from fixed slots that suit every
  palette, light and dark; strokes can be switched to a sketchy look per drawing. Changes save
  about a second after the last one ("Done" goes back to the note); if someone else saved the
  drawing meanwhile, you choose whose version stays. In a note, drawings show in place: a click
  opens the editor (readers get a large view, everyone has the button in the corner), and in
  the editor the handle at the corner sets the width (`|600`). A drawing is a plain SVG file
  that any image viewer shows, with the editable drawing stored inside it, so it is also an
  ordinary attachment for Obsidian, git or a sync tool. The graph shows drawings as small
  squares, joined by dotted lines to the notes that embed them (embeds are not synapses; the
  "Drawings" switch hides them).
- **Icons for drawings** (the picture button in the drawing editor). dendronotes brings 64 line
  icons of its own for project management, software development, cloud and networking; they take
  the line color and width like shapes, suit light and dark, and lines stick to them. Each soma
  also has a library of uploaded pictures: everyone with access uses it, everyone who may create
  files adds PNG, JPEG, WebP, GIF or SVG pictures (scaled to at most 512 pixels; SVG only
  without scripts or external references), and uploaders or editors delete them. A drawing
  keeps its own copy of the uploaded pictures it uses. A field above the icons finds one by name.
- **Downloading drawings** ("Download" in the drawing editor or the large view): the SVG file
  itself (any image viewer shows it, and it stays editable in dendronotes) or a PNG picture at
  twice its size, light or dark as the app is shown. Text in the PNG uses the fonts of the
  system.
- **Graph** (⌘⇧G / Ctrl+Shift+G, or the graph button in the left rail). All notes as
  nodes, synapses as lines, sized by their number of synapses; unresolved synapses point to
  dashed nodes. Hover a note to see its neighborhood, click to open it, drag to arrange, scroll
  or use the buttons to zoom. "Around …" shows only the neighborhood of the note opened last,
  one to three steps deep; notes without synapses and unresolved synapses can be hidden.
- **Embedded notes.** `![[Note]]` shows another note inside this one in the reading view,
  `![[Note#Heading]]` only that section (up to three levels deep).
- **Renaming and moving.** Synapses in other notes are rewritten so they keep pointing to the
  renamed or moved note (Markdown links included), and so are embeds and links to renamed or
  moved attachments (`![[photo.png]]`, `![](photo.png)`, `[report](report.pdf)`), unless
  switched off in the settings. A note that was changed by someone else in that moment is left
  alone and reported.
- **Editing together.** Everyone who has a note open (other people the soma is shared with,
  or your own second tab) edits the same text at once: changes appear right away, the others'
  cursors and selections show with their names, and the bar at the bottom of the note shows who
  else is in it. Readers follow along in the reading view without being able to change anything.
- **Saving.** The server writes a note about a second after typing stops, at the latest every
  five seconds while typing goes on ("Saving …", then "Saved"); ⌘S / Ctrl+S saves at once.
  Files keep their line breaks (also Windows-style) and a byte order mark. Without a
  connection, the status says so, the changes stay in the tab and are sent as soon as the
  connection is back, also if the server restarted meanwhile: they are merged into the current
  text.
- **History** (the clock in the top bar). Every save is kept as a version: saves by the same
  people within ten minutes count as one, changes made outside dendronotes and restored
  versions get their own. The history lists who changed what and when, shows the changes from
  any version to the current note line by line, and, for everyone who may change notes,
  brings a version back (the current text stays in the history). It follows renames and the
  trash; versions older than `DENDRONOTES_HISTORY_DAYS` (90 days) are removed, but the newest
  version of every note always stays. The history lives in `DENDRONOTES_DATA_DIR/history/`,
  never in the soma.
- **Comments** (the speech bubble in the top bar). Select text in the editor or the reading
  view and press "Comment", or comment on the whole note. Everyone with access comments and
  replies, readers too; writers mark threads as done, authors change or delete their own
  comments, editors anyone's. The commented text is marked in the note and found again after
  edits; if it is gone, the thread says so. `@user name` lets someone with access know, and
  replies notify the others in the thread: the bell in the top bar lists both and jumps to
  the comment. Comments live in `state.db`, never in the note, and follow renames and the
  trash.
- **Trash** (the bin at the bottom of the sidebar, for everyone who may delete). Deleted files
  and folders wait in the soma's `.trash` folder (the same place Obsidian uses) with who
  deleted them, when and from where. Restore puts them back where they were (missing folders
  are created again, an occupied name gets a " (n)" suffix); the owner can also delete single
  entries or everything for good, which takes their history along. After
  `DENDRONOTES_TRASH_DAYS` (7 days) entries are deleted for good on their own; for files put
  into the trash elsewhere, counted from when dendronotes first sees them. Deleted somas are not
  affected; admins restore or delete them.
- **Download as ZIP** (soma switcher). All visible notes, files and folders of the soma as they
  are now; hidden folders such as `.trash` and `.obsidian` stay out.
- **Live updates.** Changes made outside dendronotes (another editor, git, a sync tool)
  appear right away: the file tree, search and backlinks update, and an open note takes over
  the change. If it was edited here at the same time, both are merged line by line; where both
  sides changed the same lines, both versions are kept one after the other (and a notice says
  so). Nothing is lost.

| Shortcut         | Action                         |
|------------------|--------------------------------|
| ⌘O / Ctrl+O      | Quick switcher: open or create a note (Shift+Enter creates) |
| ⌘⇧F / Ctrl+Shift+F | Search notes                 |
| ⌘⇧G / Ctrl+Shift+G | Graph (again: back to the note) |
| ⌘E / Ctrl+E      | Switch between edit and read   |
| ⌘S / Ctrl+S      | Save now                       |
| ⌘, / Ctrl+,      | Settings                       |

In the drawing editor: V select, H move the canvas (or hold Space), R rectangle, O ellipse,
D diamond, L line, A arrow, P pen, T text, I icons, G grid; ⌘Z / ⇧⌘Z undo and redo, ⌘D
duplicate, ⌘C / ⌘V copy and paste (also between drawings), ⌘] / ⌘[ forward and backward
(with ⇧ to the front or back), arrow keys nudge (with ⇧ by 10), Delete removes, Enter edits the
text, ⇧1 fits the drawing, ⌘ + scroll or pinch zooms. Shift keeps proportions and 15° steps;
Alt-drag moves a copy.

**Settings** (stored in the browser, never in the soma): live preview, Vim key bindings,
readable line length, note font (sans or serif), theme (system, light, dark), color palette,
the details panel, and whether renaming updates synapses.

**Color palettes**, each with a light and a dark variant: Cortex (the default; gray matter,
graphite with a touch of rose), Sage (leaf green) and six taken from the ways nerve tissue is
made visible: the stains Golgi (ink on aged paper, silver turned gold), Nissl (cresyl violet),
Eosin (H&E: pink and blue-violet) and Myelin (Luxol fast blue with a rose counterstain), and the
fluorescence of Confocal (cyan and magenta) and Brainbow (coral and turquoise).

Uploads are limited to images (PNG, JPEG, GIF, WebP, AVIF, BMP, ICO, SVG), PDF, audio (MP3,
WAV, Ogg, FLAC, M4A), video (MP4, WebM, MOV), text (Markdown, TXT, CSV, JSON) and zip files,
up to `DENDRONOTES_MAX_UPLOAD_MB`. The content must match the extension.

## Accounts, somas and administration

- **Somas.** Every account has its own somas; new accounts start with an empty one called
  "Notes". The switcher at the top of the sidebar changes the soma ("Switch soma") and creates,
  renames or deletes one. Each soma is a folder in `DENDRONOTES_SOMAS_DIR`, named after the
  soma when it is created (renaming the soma keeps the folder). Deleting a soma moves its
  folder to `DENDRONOTES_SOMAS_DIR/.trash/`; admins find it under *Somas → Deleted somas* and
  restore it with its notes and shares, or delete it for good. A soma registered from
  `DENDRONOTES_SOMA_DIR` keeps its folder where it is.
- **Sharing** ("Share soma …" in the soma switcher). The owner adds people and groups by name,
  each with a role; every role includes the ones above it:

  | Role | May |
  |------|-----|
  | *Can read* (viewer) | read notes and files |
  | *Can write* (writer) | change existing notes and tick tasks |
  | *Can add* (contributor) | create notes, folders and files (uploads, new notes from synapses) |
  | *Full access* (editor) | rename, move and delete |
  | *Owner* | share, rename or delete the soma, hand it over ("Make owner"; the previous owner keeps full access) |

  Someone who gets a soma both personally and through groups has the highest of those roles.
  Somas shared with you are listed under "Shared with you"; "People with access …" shows who
  else is in and lets you leave a soma shared with you personally (access through a group
  stays). What a role does not allow is hidden in the interface and refused by the server.
  Changes to sharing take effect at once, also in browsers that have the soma open; someone
  who loses access is disconnected right away.
- **Profile** (your picture at the top right, which also signs you out): profile picture,
  display name, e-mail and, for accounts with a password, a new password. Changing it signs
  out your other sessions.
- **Profile pictures**: design one from a color, a pattern (dendrite, myelin rings, vesicles)
  and your initials, or upload a photo and place it in a circle. The browser crops the photo to
  a small square; the server stores it in `state.db`, encoded anew without metadata, and shows
  it only to people you share a soma with (and to admins). Pictures appear in the header (yours),
  at the bottom of a note (the others in it), in the sharing dialog and in the administration.
- **Administration** (the shield in the left rail, admins only):
  - *Users*: create accounts, change role, disable, set a new password, sign out everywhere,
    delete. A deleted account's somas go to an account you choose or to the trash. The last
    admin can be neither demoted, disabled nor deleted.
  - *Groups*: create, rename and delete groups and choose their members. The group
    *Everyone* is built in and contains every account, also future ones; share a soma with it
    to open it to the whole server. Groups taken over from Active Directory are marked as such;
    their name and members come from the directory (see [authentication](authentication.md)).
  - *Somas*: every soma with owner, members, groups, size and folder; create a soma for any
    account (optionally shared with a group right away, for example *Everyone*), hand a soma
    to another owner or delete it.
  - *Sessions*: who is signed in from where; end single sessions.
  - *Server*: version, uptime, storage, open somas, a backup download, whether the OpenID
    Connect provider and the directory are reachable, and the effective configuration
    (secrets show only whether they are set).
  - *Audit log*: sign-ins (failed ones too), account and soma changes.

Disabling an account, setting its password or ending its sessions takes effect at once; open
browser tabs notice within 25 seconds.

## Index and live updates

dendronotes keeps a search index per soma in `$DENDRONOTES_DATA_DIR/index/<soma id>/index.db`
(SQLite with FTS5, compiled into the binary, no extra service). It is a cache: delete it at
any time and it is rebuilt from the soma the next time the soma is opened, as it is when its
format changes or the file is damaged. Somas are opened on first use and closed after 30
minutes without connected browsers.

Changes on disk are picked up through file system events (inotify on Linux). Every
`DENDRONOTES_RESCAN_INTERVAL` (default 5 minutes) the whole soma is compared as well, which
covers file systems without events, such as some network volumes. A very large soma may need
a higher inotify limit on the host (`fs.inotify.max_user_watches`); without it, changes are
still found by the rescans.

Browsers receive updates through Server-Sent Events (`/api/somas/{soma}/events`). Behind a reverse
proxy, do not buffer that path and allow idle reads of at least 30 seconds (a heartbeat is
sent every 25 seconds); dendronotes sets `X-Accel-Buffering: no` for nginx.
