// Core component system
export type { Component } from './Component';
export { BaseComponent } from './Component';
export type { LCMComponent, LCMComponentConfig, LCMComponentEvent } from './LCMComponent';
export { LifecycleController } from './LifecycleController';

// Event system
export {
    EventBus,
    ComponentEventTypes,
    LifecycleEventTypes,
} from './EventBus';
export type {
    EventHandler,
    EventSubscriber,
    LifecycleEventType,
    ComponentEventType
} from './EventBus';
export type { ComponentLifecycleEvent } from './events';

// UI Components
export { BasePage } from './BasePage';
export { Modal } from './Modal';
export { ToastManager } from './ToastManager';
export { ThemeManager } from './ThemeManager';
export { TemplateLoader } from './TemplateLoader';
export { SplashScreen } from './SplashScreen';
export { MobileBottomDrawer } from './MobileBottomDrawer';

// Island pages (islands mounted from a server-written page spec)
export { IslandPage } from './page/IslandPage';
export { readSpec, SPEC_ELEMENT_ID } from './page/spec';
export type { IslandSpec, PageSpec, SpecExtension } from './page/spec';
export { mountIslands } from './page/mount';
export type { IslandFactory, MountOptions, Registry } from './page/mount';
export { parseLoad, scheduleMount } from './page/load';
export type { LoadEnv, LoadStrategy } from './page/load';

// Utilities
export { isInInputContext, hasModifierKeys, shouldIgnoreShortcut } from './DOMUtils';
export {
    KeyboardShortcutManager,
    KeyboardState,
} from './KeyboardShortcutManager';
export type {
    ShortcutConfig,
    ShortcutManagerConfig
} from './KeyboardShortcutManager';
