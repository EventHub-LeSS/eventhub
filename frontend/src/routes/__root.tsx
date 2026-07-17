import { Outlet, createRootRoute } from "@tanstack/react-router";

import { NavBar } from "@/components/NavBar";
import { Footer } from "@/components/Footer";

export const Route = createRootRoute({
  component: RootLayout,
});

function RootLayout() {
  return (
    <div className="relative flex min-h-svh flex-col">
      {/* Decorative background — sits over the base color, under all content */}
      <div
        aria-hidden
        className="pointer-events-none fixed inset-0 -z-10 bg-background"
      >
        <img
          src="/bg-lines.svg"
          alt=""
          className="h-full w-full object-cover opacity-70"
        />
      </div>

      <NavBar />
      <div className="flex-1">
        <Outlet />
      </div>
      <Footer />
    </div>
  );
}
