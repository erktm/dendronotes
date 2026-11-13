// A note on its own, outside dendronotes: its title (the file name) comes
// along as the first heading, as it shows above the text in the app.

/** The note's Markdown with the title as a level-1 heading on top. */
export function markdownWithTitle(title: string, content: string): string {
  const body = content.replace(/^﻿/, '').replace(/^(\s*\n)+/, '')
  // A note that starts with exactly this heading keeps it once.
  if (body.split('\n', 1)[0]!.trim() === `# ${title}`) return body
  return body ? `# ${title}\n\n${body}` : `# ${title}\n`
}
