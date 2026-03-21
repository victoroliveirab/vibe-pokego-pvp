import {
  Show,
  SignInButton,
  SignUpButton,
  UserButton,
} from "@clerk/react";
import { NavLink, Outlet } from "react-router-dom";

function navLinkClassName({ isActive }) {
  return [
    "rounded-full border px-3 py-2 text-xs font-semibold transition",
    isActive
      ? "border-slate-100 bg-slate-100 text-slate-950"
      : "border-slate-700 bg-slate-900/70 text-slate-200 hover:border-slate-500 hover:bg-slate-800",
  ].join(" ");
}

function authButtonClassName(kind) {
  if (kind === "primary") {
    return "rounded-full border border-cyan-300/40 bg-cyan-400/10 px-3 py-2 text-xs font-semibold text-cyan-100 transition hover:bg-cyan-400/20";
  }

  return "rounded-full border border-slate-700 bg-slate-900/70 px-3 py-2 text-xs font-semibold text-slate-200 transition hover:border-slate-500 hover:bg-slate-800";
}

export default function AppShell() {
  return (
    <div className="min-h-screen bg-gradient-to-b from-slate-950 via-slate-900 to-slate-950">
      <header className="border-b border-slate-800/80 bg-slate-950/80 backdrop-blur">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-4 sm:px-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="space-y-1">
              <NavLink className="text-lg font-semibold text-white" to="/">
                Vibe PoGo
              </NavLink>
              <p className="text-sm text-slate-300">
                Scan appraisals instantly, then sign in when you want your records to follow you.
              </p>
            </div>

            <div className="flex items-center gap-2">
              <Show when="signed-out">
                <SignInButton mode="modal">
                  <button className={authButtonClassName("secondary")} type="button">
                    Sign in
                  </button>
                </SignInButton>
                <SignUpButton mode="modal">
                  <button className={authButtonClassName("primary")} type="button">
                    Sign up
                  </button>
                </SignUpButton>
              </Show>

              <Show when="signed-in">
                <UserButton afterSignOutUrl="/" />
              </Show>
            </div>
          </div>

          <nav className="flex flex-wrap gap-2">
            <NavLink className={navLinkClassName} end to="/">
              Upload
            </NavLink>
            <NavLink className={navLinkClassName} to="/all">
              All Pokemon
            </NavLink>
          </nav>
        </div>
      </header>

      <Outlet />
    </div>
  );
}
