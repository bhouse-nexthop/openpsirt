import type { ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import { api } from "../api/client";
import type { Who } from "./session";

const places = [
  { to: "/", label: "Home", end: true },
  { to: "/products", label: "Products", end: false },
  { to: "/review-queue", label: "Review queue", end: false },
];

// The frame every screen sits in. Deliberately thin: where you are, who you
// are, and how to leave.
export function Shell({ who, children }: { who: Who; children: ReactNode }) {
  return (
    <div className="min-h-dvh">
      <header className="border-b border-edge bg-raised">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 sm:px-6">
          <Link to="/" className="flex items-center gap-2">
            <img src="/brand/mark.svg" alt="" className="h-6 w-6" />
            <span className="font-semibold tracking-tight">OpenPSIRT</span>
          </Link>

          <nav className="flex gap-1 text-sm">
            {places.map((place) => (
              <NavLink
                key={place.to}
                to={place.to}
                end={place.end}
                className={({ isActive }) =>
                  `rounded px-2 py-1 ${isActive ? "bg-sunken text-ink" : "text-muted hover:text-ink"}`
                }
              >
                {place.label}
              </NavLink>
            ))}
          </nav>

          <div className="ml-auto flex items-center gap-3">
            <span className="hidden text-sm text-muted sm:inline">{who.name}</span>
            <button
              type="button"
              onClick={async () => {
                await api.DELETE("/v1/session", {});
                // A full load rather than a route change: signing out has to
                // drop every cached answer, and the simplest way to be sure of
                // that is to start again.
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
