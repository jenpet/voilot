import { marked } from 'marked';
import DOMPurify from 'dompurify';

// Configure marked once — GFM (tables, strikethrough, autolinks) + line breaks
marked.setOptions({
  gfm: true,
  breaks: true,
});

/**
 * Parses a raw markdown string into sanitized HTML.
 * Safe for use with v-html — DOMPurify strips all dangerous content.
 */
export function renderMarkdown(raw: string): string {
  if (!raw) return '';
  const html = marked.parse(raw, { async: false }) as string;
  return DOMPurify.sanitize(html);
}
