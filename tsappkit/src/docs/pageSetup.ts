/**
 * pageSetup.ts - Common page initialization shared across all doc pages
 */

/**
 * Highlights the currently active link in the sidebar navigation
 * @param sidebarSelector - CSS selector for sidebar links (default: '.sidebar .section-nav a')
 */
export function highlightActiveSidebarLink(sidebarSelector: string = '.sidebar .section-nav a'): void {
  const currentPath = window.location.pathname;
  const sidebarLinks = document.querySelectorAll(sidebarSelector);

  sidebarLinks.forEach((link) => {
    const href = link.getAttribute('href');
    if (href && currentPath.endsWith(href.replace(/\/$/, '') + '/')) {
      link.classList.add('active');
    } else if (href && currentPath === href) {
      link.classList.add('active');
    }
  });
}

/**
 * Initialize common page features
 * Call this on DOMContentLoaded for all doc pages
 * @param options - Configuration options for page setup
 */
export function initPageSetup(options: {
  sidebarSelector?: string;
} = {}): void {
  highlightActiveSidebarLink(options.sidebarSelector);
}
