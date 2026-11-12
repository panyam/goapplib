/**
 * A LifeCycle Managed Component: one that a LifecycleController starts in
 * phases, so a component can rely on every other component being set up
 * before it activates.
 *
 * Keep the constructor to storing its arguments. The controller decides when
 * the real work happens, phase by phase over the whole tree:
 * performLocalInit, then setupDependencies, then activate. deactivate runs
 * when the controller is torn down, leaves first.
 * See https://panyam.github.io/goapplib/guide/components/.
 */
export interface LCMComponent {
    /**
     * Phase 1: set the component itself up, and return its children.
     *
     * Bind to the DOM and create child components here, without starting
     * them. The children returned become the next level of the tree. Other
     * components may not be set up yet, so don't reach for them. A Promise is
     * awaited before the next component in the level starts.
     */
    performLocalInit(): Promise<LCMComponent[]> | LCMComponent[];

    /**
     * Phase 2: take references to other components.
     *
     * Every component in the tree has finished performLocalInit by now, so a
     * parent can hand its children their siblings, or itself. The controller
     * doesn't await a Promise returned here: activate can start before it
     * resolves, so do async work in activate instead.
     */
    setupDependencies(): Promise<void> | void;

    /**
     * Phase 3: start working. Subscribe to events, load data, render.
     *
     * Every component has finished setupDependencies. A level's activate
     * calls run together, and the next level starts once all of them resolve.
     */
    activate(): Promise<void> | void;

    /**
     * Stop working and release what activate took: subscriptions, listeners,
     * connections. LifecycleController.deactivateAll calls it on the children
     * before the parent, so a component doesn't deactivate its children itself.
     */
    deactivate(): Promise<void> | void;
}

/**
 * Configuration for component lifecycle behavior
 */
export interface LCMComponentConfig {
    /**
     * Maximum time to wait for a lifecycle phase to complete (ms)
     * Default: 10000 (10 seconds)
     */
    phaseTimeoutMs?: number;

    /**
     * Whether to continue if individual components fail during a phase
     * Default: false (fail fast)
     */
    continueOnError?: boolean;

    /**
     * Whether to validate dependencies against declared requirements
     * Default: true
     */
    validateDependencies?: boolean;

    /**
     * Enable debug logging for lifecycle phases
     * Default: false
     */
    enableDebugLogging?: boolean;
}

/**
 * Event emitted during component lifecycle transitions
 */
export interface LCMComponentEvent {
    type: 'phase-start' | 'phase-complete' | 'phase-error' | 'component-ready';
    componentName: string;
    timestamp: number;
    error?: Error;
    metadata?: Record<string, any>;
}
