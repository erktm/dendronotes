// Every user-facing text of dendronotes lives here (English only), so the
// wording stays consistent and is easy to change. Vocabulary: see CLAUDE.md
// (soma = the notes folder, synapse = a connection between two notes).
// The product name is always written in lowercase: dendronotes.

export const brand = {
  name: 'dendronotes',
  /** The wordmark is drawn as an accented first part plus the rest. */
  wordmark: { accent: 'dendro', rest: 'notes' },
  tagline: 'Self-hosted Markdown notes',
} as const

/** The platform's main modifier key, for shortcut hints. */
const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)
export const modKey = isMac ? '⌘' : 'Ctrl+'
/** Modifier plus Shift, e.g. ⌘⇧F or Ctrl+Shift+F. */
export const modShiftKey = isMac ? '⌘⇧' : 'Ctrl+Shift+'

export const strings = {
  app: {
    loading: 'Loading',
    title: (note?: string) => (note ? `${note} – ${brand.name}` : brand.name),
  },

  view: {
    label: 'View',
    edit: 'Edit',
    read: 'Read',
    editHint: `Edit (${modKey}E)`,
    readHint: `Read (${modKey}E)`,
    showDetails: 'Show details panel',
    hideDetails: 'Hide details panel',
  },

  editor: {
    toggleTask: 'Toggle task',
    openSynapseHint: `${modKey}click to open`,
    lineCol: (line: number, col: number) => `Ln ${line}, Col ${col}`,
    words: (n: number) => (n === 1 ? '1 word' : `${n.toLocaleString('en')} words`),
  },

  reading: {
    attachment: (name: string) => `Attachment: ${name}`,
    externalImage: (alt: string) => `External image not loaded: ${alt}`,
    embeddedNote: 'Embedded note:',
  },

  details: {
    label: 'Note details',
    outline: 'Outline',
    noHeadings: 'No headings yet',
    outgoing: 'Synapses',
    noSynapses: 'No synapses yet',
    notCreated: 'not created yet',
    embed: 'embedded',
    incoming: 'Incoming synapses',
    noIncoming: 'No other note connects here yet',
    incomingLoading: 'Looking for synapses …',
    openAt: (name: string, line: number) => `Open ${name} at line ${line + 1}`,
  },
  search: {
    title: 'Search',
    open: `Search notes (${modShiftKey}F)`,
    label: 'Search notes',
    placeholder: 'Search notes …',
    hint: 'Words match their beginning, "quotes" an exact phrase, #tag a tag.',
    results: 'Search results',
    count: (n: number) => (n === 1 ? '1 note' : `${n} notes`),
    empty: 'No notes match',
    incomplete: 'Still indexing the soma – results may be missing.',
    tags: 'Tags',
    noTags: 'No tags yet. Write #tag in a note to add one.',
    nameMatch: 'Name matches',
    clear: 'Clear search',
  },

  switcher: {
    title: 'Open a note',
    open: `Open a note (${modKey}O)`,
    placeholder: 'Find or create a note …',
    results: 'Matching notes',
    empty: 'No matching notes',
    create: (q: string) => `Create note “${q}”`,
    inFolder: (folder: string) => (folder ? `in ${folder}` : 'in the soma'),
    navigate: 'navigate',
    openKey: 'open',
    createKey: 'create',
    close: 'close',
  },

  conflict: {
    text: 'Compare both versions and choose one.',
    yours: 'Your version',
    yoursHint: 'not saved',
    disk: 'On disk',
    diskHint: 'changed elsewhere',
    loading: 'Loading the version on disk …',
    explain: 'Both sides show their changes against the version you opened. Nothing is overwritten until you choose.',
  },

  settings: {
    title: 'Settings',
    open: `Settings (${modKey},)`,
    close: 'Close settings',
    editor: 'Editor',
    livePreview: 'Live preview',
    livePreviewDesc: 'Render Markdown as you type; the line with the cursor shows its syntax.',
    vim: 'Vim key bindings',
    vimDesc: 'Normal, insert and visual mode in the editor.',
    readable: 'Readable line length',
    readableDesc: 'Keep lines to about 70 characters.',
    noteFont: 'Note font',
    sans: 'Sans',
    serif: 'Serif',
    appearance: 'Appearance',
    theme: 'Theme',
    system: 'System',
    light: 'Light',
    dark: 'Dark',
    details: 'Show details panel',
    detailsDesc: 'Outline and synapses next to the note.',
    files: 'Files',
    updateSynapses: 'Update synapses on rename',
    updateSynapsesDesc: 'When a note is renamed or moved, rewrite the synapses in other notes so they keep pointing to it.',
    soma: 'Soma',
    stored: 'Settings are stored in this browser. Your notes stay plain Markdown files; nothing is written into the soma.',
  },

  auth: {
    pageTitle: `Sign in – ${brand.name}`,
    welcome: 'Welcome back',
    lead: 'Sign in to open your soma.',
    password: 'Password',
    signIn: 'Sign in',
    signingIn: 'Signing in …',
    wrongPassword: 'That password isn’t correct.',
    tooManyAttempts: (secs: number) => `Too many attempts. Try again in ${secs} seconds.`,
    proxyTitle: 'You’re not signed in',
    proxyText:
      'This server expects your reverse proxy (for example oauth2-proxy) to sign you in, but it ' +
      'didn’t receive a user. Reload to try again, or ask your administrator.',
    reload: 'Reload',
    sessionEnded: 'Your session has ended. Please sign in again.',
    signOut: 'Sign out',
    signedInAs: (user: string) => `Signed in as ${user}`,
  },

  nav: {
    main: 'Main navigation',
    files: 'Files',
    showFiles: 'Show files',
    hideFiles: 'Hide files',
    search: 'Search',
    settings: 'Settings',
  },

  soma: {
    label: 'Soma',
    treeLabel: 'Notes in this soma',
    empty: 'Your soma is empty.',
    createFirstNote: 'Create your first note',
    loading: 'Loading …',
    location: 'Location',
  },

  tree: {
    newNote: 'New note',
    newFolder: 'New folder',
    rename: 'Rename',
    delete: 'Delete',
    noteName: 'Note name',
    folderName: 'Folder name',
    newName: 'New name',
    readOnly: 'Read-only (symbolic link)',
    moveIntoSelf: 'A folder can’t be moved into itself.',
  },

  names: {
    empty: 'Please enter a name.',
    leadingDot: 'Names can’t start with a dot.',
    forbidden: 'Names can’t contain \\ / : * ? " < > |',
    space: 'Names can’t start or end with a space.',
    trailingDot: 'Names can’t end with a dot.',
    reserved: 'That name is reserved on Windows.',
    tooLong: 'That name is too long.',
  },

  dialogs: {
    cancel: 'Cancel',
    dismiss: 'Dismiss',
    deleteNoteTitle: 'Delete note?',
    deleteFolderTitle: 'Delete folder?',
    deleteMessage: (name: string, isFolder: boolean) =>
      `“${name}”${isFolder ? ' and everything in it' : ''} will be moved to the .trash folder in your soma.`,
    moveToTrash: 'Move to trash',
    discardTitle: 'Discard your changes?',
    discardMessage: (name: string) => `“${name}” was changed elsewhere and your version is not saved.`,
    discard: 'Discard',
  },

  toasts: {
    movedToTrash: (name: string) => `Moved “${name}” to the trash.`,
    saveFailed: (reason: string) => `Couldn’t save: ${reason}`,
    synapsesUpdated: (synapses: number, notes: number) =>
      `Updated ${synapses === 1 ? '1 synapse' : `${synapses} synapses`} in ${notes === 1 ? '1 note' : `${notes} notes`}.`,
    synapsesSkipped: (n: number) =>
      `${n === 1 ? '1 note was' : `${n} notes were`} changed meanwhile; their synapses were not updated.`,
    changedElsewhere: (name: string) => `“${name}” was updated elsewhere.`,
    deletedElsewhere: (name: string) => `“${name}” was deleted or moved elsewhere. Your unsaved changes are still in the editor.`,
  },

  empty: {
    title: 'Nothing open yet',
    text: 'Pick a note on the left or create a new one. Your notes stay plain .md files in your soma.',
  },

  note: {
    loading: 'Loading …',
    placeholder: 'Start writing …',
    contentLabel: (name: string) => `Content of ${name}`,
    notFoundTitle: 'Note not found',
    notFoundText: (name: string) => `“${name}” may have been moved or deleted.`,
    unsupportedTitle: 'Can’t open this file yet',
    unsupportedText: `${brand.name} edits text notes (.md, .markdown, .txt). Previews for other files are coming later.`,
    tooLargeTitle: 'This note is too large',
    tooLargeText: 'It exceeds the size limit for the editor (DENDRONOTES_MAX_NOTE_MB).',
    errorTitle: 'Couldn’t load this note',
    errorText: 'Something went wrong while loading it.',
    retry: 'Try again',
    conflictTitle: 'This note was changed elsewhere.',
    conflictText: 'Your latest edits are not saved yet.',
    useDisk: 'Use version on disk',
    keepMine: 'Keep mine',
    readOnly: 'This note is a symbolic link and can only be read here.',
  },

  saveState: {
    readOnly: 'Read-only',
    conflict: 'Conflict',
    saving: 'Saving …',
    failed: 'Not saved',
    edited: 'Edited',
    saved: 'Saved',
  },

  server: {
    checking: 'Connecting …',
    ready: 'Connected',
    unavailable: 'Server not ready',
    offline: 'No connection',
  },

  /** Messages for the API's error codes (see docs/openapi.yaml). */
  errors: {
    byCode: {
      network: 'Can’t reach the server. Check your connection.',
      invalid_name: 'That name isn’t allowed. Avoid \\ / : * ? " < > | and leading dots.',
      invalid_path: 'That path isn’t valid.',
      exists: 'Something with that name already exists here.',
      not_found: 'It no longer exists – it may have been moved or deleted.',
      move_into_self: 'A folder can’t be moved into itself.',
      read_only: 'This file is read-only.',
      too_large: 'This note is too large to edit here.',
      not_a_note: 'Only text notes (.md, .markdown, .txt) can be edited.',
      is_directory: 'That’s a folder, not a file.',
      not_a_directory: 'The target is not a folder.',
      cross_origin_request: 'The request was blocked as cross-origin.',
      rate_limited: 'Too many attempts. Please wait a moment.',
    } as Record<string, string>,
    withCode: (code: string) => `Something went wrong (${code}).`,
    generic: 'Something went wrong.',
  },
} as const
