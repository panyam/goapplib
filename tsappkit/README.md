# @panyam/tsappkit

TypeScript application toolkit with component lifecycle management, event system, and UI utilities.

## Installation

```bash
npm install @panyam/tsappkit
```

## Features

### Component Lifecycle System

A sophisticated component lifecycle system that eliminates initialization order dependencies and race conditions:

- **LCMComponent**: Lifecycle Managed Component interface with 4-phase initialization
- **LifecycleController**: Breadth-first component orchestration with synchronization barriers
- **BaseComponent**: Abstract base class with event bus integration

```typescript
import { BaseComponent, LCMComponent, LifecycleController, EventBus } from '@panyam/tsappkit';

class MyComponent extends BaseComponent {
    performLocalInit(): LCMComponent[] {
        // Phase 1: Initialize and return child components
        return [];
    }

    setupDependencies(): void {
        // Phase 2: Receive dependencies
    }

    activate(): void {
        // Phase 3: Component is ready
    }

    deactivate(): void {
        // Cleanup
    }
}
```

### Event System

Type-safe event bus with error isolation:

```typescript
import { EventBus, EventSubscriber } from '@panyam/tsappkit';

class MySubscriber implements EventSubscriber {
    handleBusEvent(eventType: string, data: any, subject: any, emitter: any): void {
        console.log(`Received ${eventType}:`, data);
    }
}

const eventBus = new EventBus();
eventBus.addSubscription('my-event', null, new MySubscriber());
eventBus.emit('my-event', { value: 42 }, null, this);
```

### UI Components

- **Modal**: Singleton modal dialog manager with template loading
- **ToastManager**: Toast notification system
- **ThemeManager**: Light/dark/system theme management
- **SplashScreen**: Loading screen with progress updates
- **MobileBottomDrawer**: Swipe-to-close drawer for mobile
- **TemplateLoader**: HTML template loading system

### Utilities

- **DOMUtils**: Input context detection, modifier key checks
- **KeyboardShortcutManager**: Multi-key keyboard shortcut system with state machine

## Usage with BasePage

```typescript
import { BasePage, LCMComponent } from '@panyam/tsappkit';

class MyPage extends BasePage {
    protected initializeSpecificComponents(): LCMComponent[] {
        // Initialize page-specific components
        return [];
    }

    protected bindSpecificEvents(): void {
        // Bind page-specific event handlers
    }
}

// Auto-initialize on DOM ready
BasePage.loadAfterPageLoaded('myPage', MyPage, 'MyPage');
```

## License

MIT
