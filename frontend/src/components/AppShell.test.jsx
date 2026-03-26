import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import AppShell from "./AppShell";

const buildInfoState = vi.hoisted(() => ({
  getDeployedAtISO: vi.fn(),
}));

vi.mock("@clerk/react", () => ({
  Show: ({ when, children }) => (when === "signed-out" ? <>{children}</> : null),
  SignInButton: ({ children }) => <>{children}</>,
  SignUpButton: ({ children }) => <>{children}</>,
  UserButton: () => <div>User</div>,
}));

vi.mock("../lib/build-info", () => ({
  getDeployedAtISO: () => buildInfoState.getDeployedAtISO(),
}));

function renderAppShell() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/" element={<div>Page body</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("AppShell", () => {
  beforeEach(() => {
    buildInfoState.getDeployedAtISO.mockReset();
  });

  it("renders the deployment timestamp footer when available", () => {
    buildInfoState.getDeployedAtISO.mockReturnValue("2026-03-25T14:03:11Z");

    renderAppShell();

    expect(screen.getByText("Deployed:")).toBeTruthy();
    expect(screen.getByText("2026-03-25T14:03:11Z")).toBeTruthy();
  });

  it("omits the deployment timestamp footer when unavailable", () => {
    buildInfoState.getDeployedAtISO.mockReturnValue("");

    renderAppShell();

    expect(screen.queryByText("Deployed:")).toBeNull();
  });
});
