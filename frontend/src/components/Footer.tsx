export function Footer() {
  return (
    <footer className="border-t border-border">
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-3 px-4 py-6 text-sm text-muted-foreground sm:flex-row sm:px-6">
        <a
          href=""
          className="transition-colors hover:text-foreground"
        >
          © 2026 EventHub. Alle Rechte vorbehalten.
        </a>
        <nav className="flex items-center gap-6">
          <a href="" className="transition-colors hover:text-foreground">
            Datenschutz
          </a>
          <a href="" className="transition-colors hover:text-foreground">
            Lizenz
          </a>
        </nav>
      </div>
    </footer>
  );
}
