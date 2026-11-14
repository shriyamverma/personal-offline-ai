export type Appearance = "light" | "dark" | "system";
export type ColorMode = "light" | "dark";
export type ThemeId =
  | "forest"
  | "indigo"
  | "graphite"
  | "paper"
  | "nord"
  | "mono";

export interface ThemeColors {
  bg: string;
  panel: string;
  side: string;
  border: string;
  muted: string;
  hover: string;
  ink: string;
  accent: string;
  primary: string;
  "primary-hover": string;
  "primary-ink": string;
}
export interface ThemePalette {
  id: ThemeId;
  name: string;
  colors: Record<ColorMode, ThemeColors>;
}

// Change these to set the initial palettes for browsers without saved choices.
export const DEFAULT_LIGHT_THEME: ThemeId = "nord";
export const DEFAULT_DARK_THEME: ThemeId = "indigo";

// All palette colors, including buttons, live here. Each palette supports both modes.
export const PALETTES: readonly ThemePalette[] = [
  {
    id: "forest",
    name: "Forest",
    colors: {
      light: {
        bg: "#fafbf8",
        panel: "#ffffff",
        side: "#f1f3ed",
        border: "#dde3d9",
        muted: "#657269",
        hover: "#e8ede4",
        ink: "#29312f",
        accent: "#36765c",
        primary: "#256347",
        "primary-hover": "#1a4e36",
        "primary-ink": "#ffffff",
      },
      dark: {
        bg: "#151c18",
        panel: "#1c2520",
        side: "#111914",
        border: "#303e34",
        muted: "#96a69b",
        hover: "#28362d",
        ink: "#e0e7e1",
        accent: "#92c9aa",
        primary: "#92c9aa",
        "primary-hover": "#b3ddc5",
        "primary-ink": "#14261b",
      },
    },
  },
  {
    id: "indigo",
    name: "Midnight Indigo",
    colors: {
      light: {
        bg: "#f7f8ff",
        panel: "#ffffff",
        side: "#eef0fc",
        border: "#dce0f3",
        muted: "#646d88",
        hover: "#e8ebfa",
        ink: "#202840",
        accent: "#4f46e5",
        primary: "#4f46e5",
        "primary-hover": "#4338ca",
        "primary-ink": "#ffffff",
      },
      dark: {
        bg: "#0f172a",
        panel: "#1e293b",
        side: "#0b1220",
        border: "#334155",
        muted: "#94a3b8",
        hover: "#273449",
        ink: "#e2e8f0",
        accent: "#a5b4fc",
        primary: "#4f46e5",
        "primary-hover": "#4338ca",
        "primary-ink": "#ffffff",
      },
    },
  },
  {
    id: "graphite",
    name: "Warm Graphite",
    colors: {
      light: {
        bg: "#faf8f4",
        panel: "#fffdf9",
        side: "#f0ebe3",
        border: "#dfd7ca",
        muted: "#756955",
        hover: "#eee5d7",
        ink: "#302b24",
        accent: "#946215",
        primary: "#86580d",
        "primary-hover": "#6e4609",
        "primary-ink": "#ffffff",
      },
      dark: {
        bg: "#211e1a",
        panel: "#2c2721",
        side: "#191713",
        border: "#473e31",
        muted: "#b5a894",
        hover: "#393126",
        ink: "#f0e7d7",
        accent: "#e6b96b",
        primary: "#e6b96b",
        "primary-hover": "#f0ca89",
        "primary-ink": "#2b2112",
      },
    },
  },
  {
    id: "paper",
    name: "Paper & Ink",
    colors: {
      light: {
        bg: "#faf7f0",
        panel: "#fffdf7",
        side: "#efebe1",
        border: "#dcd7ca",
        muted: "#6b6c68",
        hover: "#e8e5dc",
        ink: "#252a30",
        accent: "#365c7e",
        primary: "#365c7e",
        "primary-hover": "#284760",
        "primary-ink": "#ffffff",
      },
      dark: {
        bg: "#202428",
        panel: "#2a3036",
        side: "#191d21",
        border: "#414951",
        muted: "#a9b2b9",
        hover: "#353d45",
        ink: "#f0eee8",
        accent: "#9bbdd6",
        primary: "#9bbdd6",
        "primary-hover": "#b8d1e3",
        "primary-ink": "#192b38",
      },
    },
  },
  {
    id: "nord",
    name: "Nordic Blue",
    colors: {
      light: {
        bg: "#f3f6f8",
        panel: "#ffffff",
        side: "#e8eef2",
        border: "#d0dbe2",
        muted: "#5c7080",
        hover: "#dfe9ef",
        ink: "#2e3440",
        accent: "#38677c",
        primary: "#38677c",
        "primary-hover": "#2b5264",
        "primary-ink": "#ffffff",
      },
      dark: {
        bg: "#2e3440",
        panel: "#3b4252",
        side: "#252b36",
        border: "#4c566a",
        muted: "#b6c2d2",
        hover: "#434c5e",
        ink: "#eceff4",
        accent: "#88c0d0",
        primary: "#88c0d0",
        "primary-hover": "#a4d2df",
        "primary-ink": "#21313a",
      },
    },
  },
  {
    id: "mono",
    name: "Monochrome",
    colors: {
      light: {
        bg: "#fafafa",
        panel: "#ffffff",
        side: "#f0f0f0",
        border: "#dddddd",
        muted: "#666666",
        hover: "#e8e8e8",
        ink: "#202020",
        accent: "#404040",
        primary: "#202020",
        "primary-hover": "#404040",
        "primary-ink": "#ffffff",
      },
      dark: {
        bg: "#171717",
        panel: "#222222",
        side: "#101010",
        border: "#393939",
        muted: "#a3a3a3",
        hover: "#303030",
        ink: "#eeeeee",
        accent: "#d4d4d4",
        primary: "#eeeeee",
        "primary-hover": "#d4d4d4",
        "primary-ink": "#171717",
      },
    },
  },
];

export function isThemeId(value: unknown): value is ThemeId {
  return PALETTES.some((palette) => palette.id === value);
}
export function getPalette(id: ThemeId): ThemePalette {
  return PALETTES.find((palette) => palette.id === id)!;
}
export function resolveMode(
  appearance: Appearance,
  systemDark: boolean,
): ColorMode {
  return appearance === "system" ? (systemDark ? "dark" : "light") : appearance;
}
