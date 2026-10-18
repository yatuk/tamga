// Files a person attaches: which ones are read, how, and what is sent to the
// scan API for each. No DOM and no chrome.* here, only File objects.
//
// A text file (by its name, or by its type when the name says nothing) of up
// to FILE_MAX_BYTES is read in the content script and its text is checked
// like a typed message. Every other file (PDF, images, office documents,
// archives) is not read: the extension cannot get text out of it.

// The scan API reads a body of at most 256 KB, and JSON escaping makes the
// body larger than the file. 200 KB leaves room for that.
export const FILE_MAX_BYTES = 200 * 1024;
// One attach of more text files than this is not checked file by file: each
// is a scan, and one tab gets 30 a minute.
export const FILE_MAX_COUNT = 10;
const NAME_MAX_CHARS = 120;

export const TEXT_EXTENSIONS = Object.freeze([
  // text and data
  'txt', 'text', 'md', 'markdown', 'rst', 'adoc', 'csv', 'tsv', 'json', 'jsonl', 'ndjson', 'yaml', 'yml', 'xml',
  'log', 'ini', 'cfg', 'conf', 'toml', 'env', 'properties', 'sql', 'tex', 'srt', 'vtt', 'diff', 'patch',
  'html', 'htm', 'css', 'scss', 'less', 'svg',
  // source code
  'js', 'mjs', 'cjs', 'jsx', 'ts', 'tsx', 'vue', 'svelte', 'py', 'rb', 'go', 'rs', 'java', 'kt', 'kts', 'scala',
  'groovy', 'gradle', 'c', 'h', 'cc', 'cpp', 'cxx', 'hpp', 'cs', 'fs', 'swift', 'm', 'php', 'pl', 'pm', 'r',
  'lua', 'dart', 'ex', 'exs', 'erl', 'hs', 'clj', 'sh', 'bash', 'zsh', 'fish', 'ps1', 'psm1', 'bat', 'cmd',
  'tf', 'hcl', 'proto', 'graphql', 'gql',
]);

// Files that are text and usually have no extension.
const TEXT_NAMES = Object.freeze(['dockerfile', 'makefile', 'jenkinsfile', 'procfile', 'readme', 'license', 'authors']);

const TEXT_TYPES = Object.freeze([
  'application/json', 'application/ld+json', 'application/x-ndjson', 'application/xml', 'application/yaml',
  'application/x-yaml', 'application/toml', 'application/sql', 'application/javascript', 'application/x-sh',
]);

/** The lower-case extension of a file name, or '' when it has none. */
export function extensionOf(name) {
  const base = String(name ?? '').split(/[\\/]/).pop().toLowerCase();
  const dot = base.lastIndexOf('.');
  return dot >= 0 && dot < base.length - 1 ? base.slice(dot + 1) : '';
}

const KNOWN_BINARY = Object.freeze([
  'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'odt', 'ods', 'odp', 'rtf', 'pages', 'numbers', 'key',
  'png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'tif', 'tiff', 'heic', 'ico', 'mp3', 'wav', 'm4a', 'mp4', 'mov',
  'webm', 'zip', 'gz', 'tgz', 'rar', '7z', 'tar', 'exe', 'dll', 'bin', 'iso', 'dmg', 'epub',
]);

/**
 * Whether the extension reads this file. By its extension when the lists
 * know it, so that a file named .pdf is not read whatever type the browser
 * gives it; by its type only when they do not (no extension, or one neither
 * list has).
 */
export function isTextFile(file) {
  const name = String(file?.name ?? '');
  const ext = extensionOf(name);
  if (TEXT_EXTENSIONS.includes(ext)) return true;
  if (KNOWN_BINARY.includes(ext)) return false;
  if (ext === '' && TEXT_NAMES.includes(name.split(/[\\/]/).pop().toLowerCase())) return true;
  const type = String(file?.type ?? '').toLowerCase().split(';')[0].trim();
  return type.startsWith('text/') || TEXT_TYPES.includes(type);
}

/**
 * A file name as it may be shown and sent: no control or direction-override
 * characters (a name could otherwise be made to read as another), and not
 * longer than NAME_MAX_CHARS.
 */
export function safeName(name) {
  const clean = String(name ?? '')
    .split(/[\\/]/)
    .pop()
    .replace(/[\u0000-\u001f\u007f\u200e\u200f\u202a-\u202e\u2066-\u2069]/g, '')
    .trim();
  if (clean === '') return 'file';
  return clean.length > NAME_MAX_CHARS ? `${clean.slice(0, NAME_MAX_CHARS - 1)}\u2026` : clean;
}

/**
 * Reads a text file. Returns { ok: true, text } or { ok: false, reason }
 * with reason 'file_too_large' or 'file_unreadable'. It does not throw.
 *
 * Read as UTF-8, with anything that is not valid UTF-8 replaced, so that a
 * CSV saved in a Windows code page is still checked. A file holding NUL
 * characters is not text, whatever its name says.
 */
export async function readTextFile(file) {
  if (!file || typeof file.size !== 'number' || file.size > FILE_MAX_BYTES) return { ok: false, reason: 'file_too_large' };
  let text;
  try {
    const bytes = await file.arrayBuffer();
    text = new TextDecoder('utf-8').decode(bytes);
  } catch {
    return { ok: false, reason: 'file_unreadable' };
  }
  if (text.includes('\u0000')) return { ok: false, reason: 'file_unreadable' };
  return { ok: true, text };
}

/**
 * What is sent to the scan API for a file. The API takes one text and no
 * other field, so the file name goes in as the first line: the policy sees
 * it, and a name that holds a finding is found too.
 */
export function scanTextFor(name, text) {
  return `Attached file: ${name}\n\n${text}`;
}

/** What tells one picked file from another, without reading it. */
export function signature(file) {
  return [file?.name, file?.size, file?.lastModified, file?.type].join('\u0000');
}

/** The files of a FileList, an array, or nothing. */
export function filesOf(list) {
  if (!list || typeof list.length !== 'number') return [];
  return Array.from(list).filter((f) => f && typeof f.name === 'string');
}
