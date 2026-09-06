/** Only same-origin relative paths, so ?returnTo= cannot be used as an open redirect. */
export function safeReturnTo(value: string | null | undefined): string {
  if (!value || !value.startsWith("/")) {
    return "/"
  }

  // "//evil.com" and "/\evil.com" are protocol-relative URLs for browsers.
  if (value.startsWith("//") || value.startsWith("/\\")) {
    return "/"
  }

  return value
}
