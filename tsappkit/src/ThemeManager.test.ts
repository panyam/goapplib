import { afterEach, describe, expect, it, vi } from "vitest";
import { ThemeManager } from "./ThemeManager";

const root = document.documentElement;

function prefersDark(dark: boolean) {
  vi.stubGlobal("matchMedia", (q: string) => ({ matches: dark && q.includes("dark"), media: q }));
}

afterEach(() => {
  localStorage.clear();
  root.classList.remove("dark");
  root.removeAttribute("data-theme");
  vi.unstubAllGlobals();
});

describe("ThemeManager sets data-theme beside the dark class", () => {
  it.each([
    ["dark", false, "dark"],
    ["light", true, "light"],
    ["system", true, "dark"],
    ["system", false, "light"],
  ])("setTheme(%s) with an OS that prefers dark: %s", (setting, osDark, want) => {
    prefersDark(osDark);
    ThemeManager.setTheme(setting);
    expect(root.getAttribute("data-theme")).toBe(want);
    expect(root.classList.contains("dark")).toBe(want === "dark");
  });

  it("initialize resolves a saved choice over the OS", () => {
    prefersDark(true);
    localStorage.setItem("theme", "light");
    ThemeManager.initialize();
    expect(root.getAttribute("data-theme")).toBe("light");
    expect(root.classList.contains("dark")).toBe(false);
  });

  it("initialize follows the OS with nothing saved", () => {
    prefersDark(true);
    ThemeManager.initialize();
    expect(root.getAttribute("data-theme")).toBe("dark");
  });
});
