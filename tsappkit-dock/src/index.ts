export { createDock, type Dock, type DockOptions, type MoveInfo, type PanelLocation } from "./dock.js";
export {
  documentTheme,
  loadLayout,
  panelsToPrune,
  panelsToReconcile,
  resolveTheme,
  saveLayout,
  type DockStorage,
  type LoadedLayout,
  type PanelDef,
  type PanelPosition,
  type PanelSpec,
  type Placement,
  type SavedDrawer,
  type SavedDrawers,
} from "./layout.js";
export type { DrawerConfig } from "./drawerView.js";
export {
  clampSize,
  closesOnRelease,
  drawerPixels,
  drawerSize,
  loadDrawerSize,
  saveDrawerSize,
  DRAWER_DEFAULT,
  DRAWER_FLICK,
  DRAWER_MAX_FRAC,
  DRAWER_MIN,
  type DrawerBounds,
  type DrawerEdge,
  type DrawerStorage,
} from "./drawer.js";
