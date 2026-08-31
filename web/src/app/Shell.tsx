import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Who } from "./session";

// The frame every screen sits in. Deliberately thin: the header says who you
// are and how to leave, and everything else is the screen's own.
export function Shell({ who, children }: { who: Who; children: ReactNode }) {
  return (
    <div className="min-h-dvh">
      <header className="border-b border-edge bg-raised">
        <div className="mx-auto flex max-w-7xl items-center gap-4 px-4 py-3 sm:px-6">
          <Link to="/products" className="flex items-center gap-2">
            <img src="/brand/mark.svg" alt="" className="h-6 w-6" />
            <span className="font-semibold tracking-tight">OpenPSIRT</span>
          </Link>
          <div className="ml-auto flex items-center gap-3">
            <span className="hidden text-sm text-muted sm:inline">{who.name}</span>
            <button
              type="button"
              onClick={async () => {
                await api.DELETE("/v1/session", {});
                // A full load rather than a route change: signing out has to
                // drop every cached answer, and the simplest way to be sure
                // of that is to start again.
                window.location.assign("/");
              }}
              className="rounded border border-edge px-2 py-1 text-sm text-muted hover:text-ink"
            >
              Sign out
            </button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6">{children}</main>
    </div>
  );
}
