import { describe, it, expect, beforeEach } from 'vitest';
import { TemplateLoader } from './TemplateLoader';

describe('TemplateLoader.loadInto', () => {
  beforeEach(() => {
    document.body.replaceChildren();
    const registry = document.createElement('div');
    registry.id = 'template-registry';
    document.body.appendChild(registry);
  });

  it('names a missing template as text, so an id with markup adds no elements', () => {
    const target = document.createElement('div');
    const id = 'x<img src=x onerror=alert(1)>';
    expect(new TemplateLoader().loadInto(id, target)).toBe(false);
    expect(target.querySelector('img')).toBeNull();
    expect(target.textContent).toBe(`Error loading template '${id}' (Not Found)`);
  });
});

describe('TemplateLoader lookup', () => {
  it('finds a template whose id holds a quote', () => {
    document.body.replaceChildren();
    const registry = document.createElement('div');
    registry.id = 'template-registry';
    const wrapper = document.createElement('div');
    wrapper.dataset.templateId = 'say "hi"';
    wrapper.innerHTML = '<p>hi</p>';
    registry.appendChild(wrapper);
    document.body.appendChild(registry);
    expect(new TemplateLoader().loadHtml('say "hi"')).toBe('<p>hi</p>');
  });
});
