/**
 * Documentation site components and utilities
 *
 * This module provides reusable components for building documentation sites:
 * - CodeBlock: Copy-to-clipboard for code blocks
 * - pageSetup: Common page initialization (sidebar highlighting, etc.)
 * - ActionCompiler: Compile and run user-provided action code
 */

export { CodeBlock, initCodeBlocks } from './CodeBlock';
export { highlightActiveSidebarLink, initPageSetup } from './pageSetup';
export { ActionCompiler } from './ActionCompiler';
export type { ActionCompileResult, ActionRunResult } from './ActionCompiler';
