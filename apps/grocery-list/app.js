// Grocery List — Data Layer
// Items: {id, name, category, checked, createdAt}
// Persistence: localStorage with JSON serialization

'use strict';

const STORAGE_KEY = 'grocery-list-items';

const CATEGORIES = Object.freeze([
  'produce',
  'dairy',
  'meat',
  'bakery',
  'frozen',
  'beverages',
  'pantry',
  'other',
]);

/**
 * Load items from localStorage.
 * Returns an array of item objects, or an empty array if nothing is stored
 * or the stored data is corrupt.
 */
function loadItems() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === null) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed;
  } catch {
    console.error('Failed to load items from localStorage — returning empty list');
    return [];
  }
}

/**
 * Save items array to localStorage.
 * @param {Array} items
 */
function saveItems(items) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
}

/**
 * Create a new item object.
 * @param {string} name
 * @param {string} category — must be one of CATEGORIES
 * @returns {Object} item
 */
function createItem(name, category) {
  if (!name || typeof name !== 'string') {
    throw new Error('Item name is required and must be a string');
  }
  const cat = (category || 'other').toLowerCase();
  if (!CATEGORIES.includes(cat)) {
    throw new Error(`Invalid category "${cat}". Must be one of: ${CATEGORIES.join(', ')}`);
  }
  return {
    id: crypto.randomUUID(),
    name: name.trim(),
    category: cat,
    checked: false,
    createdAt: new Date().toISOString(),
  };
}

/**
 * Toggle the checked state of an item by id.
 * Mutates in place and returns the updated array.
 * @param {Array} items
 * @param {string} id
 * @returns {Array} items (same reference)
 */
function toggleItem(items, id) {
  const item = items.find((i) => i.id === id);
  if (item) item.checked = !item.checked;
  return items;
}

/**
 * Remove an item by id.
 * @param {Array} items
 * @param {string} id
 * @returns {Array} new array without the item
 */
function removeItem(items, id) {
  return items.filter((i) => i.id !== id);
}

// --- Theme management ---
const THEME_KEY = 'grocery-list-theme';

/** Whether the user has explicitly chosen a theme (vs. using OS default). */
function hasExplicitPreference() {
  return localStorage.getItem(THEME_KEY) !== null;
}

/**
 * Initialise theme from localStorage or system preference.
 * Sets the data-theme attribute on <html> and updates meta theme-color.
 */
function initTheme() {
  const stored = localStorage.getItem(THEME_KEY);
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const theme = stored || (prefersDark ? 'dark' : 'light');
  applyTheme(theme);
}

/**
 * Apply a theme (no transition — used for initial load and OS-change).
 * @param {'light'|'dark'} theme
 */
function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);

  // Update meta theme-color for mobile browser chrome
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) {
    meta.setAttribute('content', theme === 'dark' ? '#1a1714' : '#faf8f5');
  }
}

/**
 * Toggle between light and dark themes with a smooth CSS transition.
 * Adds a transient class so every element crossfades, then removes it
 * after the animation completes to avoid interfering with hover/focus
 * transitions during normal use.
 */
function toggleTheme() {
  const root = document.documentElement;
  const next = root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';

  // Enable transition class, apply theme, persist choice
  root.classList.add('theme-transition');
  applyTheme(next);
  localStorage.setItem(THEME_KEY, next);

  // Remove the transition class after the animation finishes
  window.setTimeout(() => root.classList.remove('theme-transition'), 400);
}

/**
 * Listen for OS-level theme changes.
 * Only follows the OS when the user hasn't made an explicit choice.
 */
function watchSystemTheme() {
  const mq = window.matchMedia('(prefers-color-scheme: dark)');
  mq.addEventListener('change', (e) => {
    if (!hasExplicitPreference()) {
      applyTheme(e.matches ? 'dark' : 'light');
    }
  });
}

// --- App state ---
let items = [];

// --- Category display names ---
const CATEGORY_LABELS = Object.freeze({
  produce: 'Produce',
  dairy: 'Dairy',
  meat: 'Meat',
  bakery: 'Bakery',
  frozen: 'Frozen',
  beverages: 'Beverages',
  pantry: 'Pantry',
  other: 'Other',
});

// --- Collapsed state (not persisted — resets on reload) ---
const collapsed = new Set();

// --- DOM references ---
let $categoryList, $addForm, $itemInput, $categorySelect, $itemCount, $themeToggle;

// --- Rendering ---

/**
 * Group items by category, returning only non-empty groups.
 * Within each group, unchecked items come first, then checked.
 * @param {Array} allItems
 * @returns {Array<{category: string, items: Array}>}
 */
function groupByCategory(allItems) {
  const groups = new Map();
  for (const cat of CATEGORIES) {
    groups.set(cat, []);
  }
  for (const item of allItems) {
    const bucket = groups.get(item.category) || groups.get('other');
    bucket.push(item);
  }
  const result = [];
  for (const [cat, catItems] of groups) {
    catItems.sort((a, b) => {
      if (a.checked !== b.checked) return a.checked ? 1 : -1;
      return a.createdAt.localeCompare(b.createdAt);
    });
    result.push({ category: cat, items: catItems });
  }
  return result;
}

/** Remove all children from a DOM node. */
function clearChildren(el) {
  while (el.firstChild) el.removeChild(el.firstChild);
}

/**
 * Render the full item list from current state.
 */
function render() {
  const groups = groupByCategory(items);
  const total = items.length;
  const checked = items.filter((i) => i.checked).length;

  // Update count with descriptive summary
  if (total === 0) {
    $itemCount.textContent = 'No items yet';
  } else if (checked === total) {
    $itemCount.textContent = `All ${total} done \u2014 nice!`;
  } else {
    const remaining = total - checked;
    $itemCount.textContent = `${remaining} remaining \u00B7 ${checked} done`;
  }

  // Build category sections
  clearChildren($categoryList);

  // Show global empty state when no items at all
  if (total === 0) {
    const emptyDiv = document.createElement('div');
    emptyDiv.className = 'empty-state';

    const icon = document.createElement('span');
    icon.className = 'empty-state-icon';
    icon.setAttribute('aria-hidden', 'true');
    icon.textContent = '\uD83D\uDED2';

    const title = document.createElement('p');
    title.className = 'empty-state-title';
    title.textContent = 'Your list is empty';

    const subtitle = document.createElement('p');
    subtitle.className = 'empty-state-subtitle';
    subtitle.textContent = 'Add your first item below to get started';

    emptyDiv.append(icon, title, subtitle);
    $categoryList.appendChild(emptyDiv);
    return;
  }

  for (const group of groups) {
    // Skip empty categories to reduce clutter
    if (group.items.length === 0) continue;

    const isCollapsed = collapsed.has(group.category);
    const section = document.createElement('section');
    section.className = 'category-section';

    const checkedInGroup = group.items.filter((i) => i.checked).length;

    // Category header (toggle collapse)
    const header = document.createElement('button');
    header.type = 'button';
    header.className = 'category-header';
    header.setAttribute('aria-expanded', String(!isCollapsed));

    const chevron = document.createElement('span');
    chevron.className = 'category-chevron' + (isCollapsed ? ' collapsed' : '');
    chevron.textContent = '\u25BC';

    const catName = document.createElement('span');
    catName.className = 'category-name';
    catName.textContent = CATEGORY_LABELS[group.category] || group.category;

    const catCount = document.createElement('span');
    catCount.className = 'category-count';
    catCount.textContent = `${checkedInGroup}/${group.items.length}`;

    const progress = document.createElement('span');
    progress.className = 'category-progress';
    const progressBar = document.createElement('span');
    progressBar.className = 'category-progress-bar';
    const pct = (checkedInGroup / group.items.length) * 100;
    progressBar.style.width = pct + '%';
    progress.appendChild(progressBar);

    header.append(chevron, catName, progress, catCount);
    header.addEventListener('click', () => {
      if (collapsed.has(group.category)) {
        collapsed.delete(group.category);
      } else {
        collapsed.add(group.category);
      }
      render();
    });
    section.appendChild(header);

    // Items list (hidden when collapsed)
    if (!isCollapsed) {
      const list = document.createElement('ul');
      list.className = 'item-list';
      for (const item of group.items) {
        list.appendChild(renderItem(item));
      }
      section.appendChild(list);
    }

    $categoryList.appendChild(section);
  }

  // "Clear completed" button when there are checked items
  if (checked > 0) {
    const clearBtn = document.createElement('button');
    clearBtn.type = 'button';
    clearBtn.className = 'clear-checked-btn';

    const checkIcon = document.createElement('span');
    checkIcon.setAttribute('aria-hidden', 'true');
    checkIcon.textContent = '\u2713';

    const btnText = document.createTextNode(` Clear ${checked} completed item${checked !== 1 ? 's' : ''}`);
    clearBtn.append(checkIcon, btnText);
    clearBtn.setAttribute('aria-label', `Clear ${checked} completed items`);
    clearBtn.addEventListener('click', () => {
      // Animate checked items out before removing
      const checkedRows = $categoryList.querySelectorAll('.item-row.checked');
      if (checkedRows.length === 0) {
        items = items.filter((i) => !i.checked);
        saveItems(items);
        render();
        return;
      }
      let finished = 0;
      checkedRows.forEach((row, i) => {
        row.style.animationDelay = `${i * 0.04}s`;
        row.classList.add('clearing');
        row.addEventListener('animationend', () => {
          finished++;
          if (finished === checkedRows.length) {
            items = items.filter((i) => !i.checked);
            saveItems(items);
            render();
          }
        }, { once: true });
      });
    });
    $categoryList.appendChild(clearBtn);
  }
}

/**
 * Render a single item row with animated interactions.
 * @param {Object} item
 * @returns {HTMLLIElement}
 */
function renderItem(item) {
  const li = document.createElement('li');
  li.className = 'item-row' + (item.checked ? ' checked' : '');
  li.dataset.id = item.id;

  const checkbox = document.createElement('input');
  checkbox.type = 'checkbox';
  checkbox.className = 'item-checkbox';
  checkbox.checked = item.checked;
  checkbox.setAttribute('aria-label', `Mark ${item.name} as ${item.checked ? 'not done' : 'done'}`);
  checkbox.addEventListener('change', () => {
    // Pop animation on the checkbox
    checkbox.classList.add('popping');
    checkbox.addEventListener('animationend', () => checkbox.classList.remove('popping'), { once: true });

    toggleItem(items, item.id);
    saveItems(items);
    render();
  });

  const name = document.createElement('span');
  name.className = 'item-name';
  name.textContent = item.name;

  const del = document.createElement('button');
  del.type = 'button';
  del.className = 'item-delete';
  del.setAttribute('aria-label', `Delete ${item.name}`);
  del.textContent = '\u00D7';
  del.addEventListener('click', () => {
    // Animate out, then remove
    li.classList.add('removing');
    li.addEventListener('animationend', () => {
      items = removeItem(items, item.id);
      saveItems(items);
      render();
    }, { once: true });
  });

  li.append(checkbox, name, del);
  return li;
}

// --- Add item handler ---

function handleAdd(e) {
  e.preventDefault();
  const name = $itemInput.value.trim();
  if (!name) return;
  try {
    const item = createItem(name, $categorySelect.value);
    items.push(item);
    saveItems(items);
    $itemInput.value = '';
    $itemInput.focus();
    render();
  } catch (err) {
    console.error('Failed to add item:', err.message);
  }
}

// --- Bootstrap ---

// Apply theme immediately (before DOMContentLoaded) to prevent flash
initTheme();
watchSystemTheme();

document.addEventListener('DOMContentLoaded', () => {
  $categoryList = document.getElementById('category-list');
  $addForm = document.getElementById('add-form');
  $itemInput = document.getElementById('item-input');
  $categorySelect = document.getElementById('category-select');
  $itemCount = document.getElementById('item-count');
  $themeToggle = document.getElementById('theme-toggle');

  items = loadItems();
  $addForm.addEventListener('submit', handleAdd);
  $themeToggle.addEventListener('click', toggleTheme);
  render();
});
