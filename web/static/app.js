const qs = (selector, root = document) => root.querySelector(selector);
const qsa = (selector, root = document) => [...root.querySelectorAll(selector)];

let currentContextBookmarkId = null;
let currentContextGroupId = null;
let currentContextGroupName = '';
let currentContextSearchEngineId = null;
let currentContextSearchEngineName = '';
let currentContextSearchEngineTemplate = '';
let activeGroupId = null;
const activeGroupStorageKey = 'tabhub-active-group-id';
let bookmarkIconPreviewURL = '';

function getStoredActiveGroupId() {
  try {
    return localStorage.getItem(activeGroupStorageKey) || '';
  } catch {
    return '';
  }
}

function storeActiveGroupId(groupId) {
  try {
    if (groupId) {
      localStorage.setItem(activeGroupStorageKey, groupId);
    } else {
      localStorage.removeItem(activeGroupStorageKey);
    }
  } catch {}
}

function toast(message) {
  const el = document.createElement('div');
  el.className = 'toast';
  el.textContent = message;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 2200);
}

function tickClock() {
  const clock = qs('#clock');
  const date = qs('#date');
  if (!clock) return;
  const now = new Date();
  clock.textContent = now.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  if (date) {
    const year = now.getFullYear();
    const dateText = now.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric', weekday: 'long' });
    date.textContent = `${year}年 ${dateText}`;
  }
}
setInterval(tickClock, 1000);
tickClock();

function openModal(id) {
  const dialog = qs(id);
  if (!dialog || dialog.open) return;
  dialog.showModal();
}
function closeAllModals() {
  qsa('dialog').forEach(dialog => {
    if (dialog.id !== 'login-modal') dialog.close();
  });
}
function hideContextMenus() {
  qs('#bookmark-context-menu')?.classList.add('hidden');
  qs('#group-context-menu')?.classList.add('hidden');
  qs('#search-engine-context-menu')?.classList.add('hidden');
  currentContextBookmarkId = null;
  currentContextGroupId = null;
  currentContextGroupName = '';
  currentContextSearchEngineId = null;
  currentContextSearchEngineName = '';
  currentContextSearchEngineTemplate = '';
}
function closeSearchEngineDock() {
  qs('#search-engine-dock')?.classList.remove('is-open');
}
function toggleSearchEngineDock() {
  qs('#search-engine-dock')?.classList.toggle('is-open');
}
function setSubmitting(form, submitting) {
  const submitButton = qs('button[type="submit"]', form);
  if (!submitButton) return;
  submitButton.disabled = submitting;
  submitButton.dataset.originalText ||= submitButton.textContent;
  submitButton.textContent = submitting ? '处理中...' : submitButton.dataset.originalText;
}

function setButtonSubmitting(button, submitting) {
  if (!button) return;
  button.disabled = submitting;
  button.dataset.originalText ||= button.textContent;
  button.textContent = submitting ? '处理中...' : button.dataset.originalText;
}

function setBookmarkIconPreview(src = '', label = '') {
  const preview = qs('#bookmark-icon-preview');
  if (!preview) return;
  if (bookmarkIconPreviewURL) {
    URL.revokeObjectURL(bookmarkIconPreviewURL);
    bookmarkIconPreviewURL = '';
  }
  preview.innerHTML = '';
  if (src) {
    const img = document.createElement('img');
    img.src = src;
    img.alt = label || '图标';
    preview.appendChild(img);
    return;
  }
  const span = document.createElement('span');
  span.textContent = (label || '图').slice(0, 1);
  preview.appendChild(span);
}

async function send(url, method, body) {
  const response = await fetch(url, { method, body, headers: { 'X-Requested-With': 'fetch' } });
  if (response.status === 401) {
    qs('#login-modal')?.showModal();
    throw new Error('请先登录');
  }
  if (!response.ok) {
    let message = '请求失败';
    const clone = response.clone();
    try {
      const data = await response.json();
      if (data.error) message = data.error;
    } catch {
      try {
        const text = await clone.text();
        if (text?.trim()) message = text.trim().slice(0, 200);
      } catch {}
    }
    throw new Error(message);
  }
  return response.json().catch(() => ({}));
}

function openBookmarkModal(groupId = '', bookmark = null) {
  const form = qs('#bookmark-form');
  form.reset();
  qs('[name=id]', form).value = bookmark?.id || '';
  qs('[name=title]', form).value = bookmark?.title || '';
  qs('[name=url]', form).value = bookmark?.url || '';
  qs('[name=group_id]', form).value = bookmark?.groupId || groupId || activeGroupId || '';
  setBookmarkIconPreview(bookmark?.iconSrc || '', bookmark?.title || '');
  openModal('#bookmark-modal');
  qs('[name=title]', form)?.focus();
}

function openSearchEngineModal(engine = null) {
  closeSearchEngineDock();
  const form = qs('#search-engine-form');
  form.reset();
  qs('[name=id]', form).value = engine?.id || '';
  qs('[name=name]', form).value = engine?.name || '';
  qs('[name=url_template]', form).value = engine?.template || '';
  openModal('#search-engine-modal');
  qs('[name=name]', form)?.focus();
}

function setActiveSearchEngineTile(tile) {
  qsa('.search-engine-tile[data-search-engine-id]').forEach(item => {
    item.classList.toggle('is-active', item === tile);
    item.dataset.searchEngineActive = item === tile ? 'true' : 'false';
  });
  const sourceIcon = qs('.search-engine-tile-icon', tile);
  const targetIcon = qs('#search-engine-icon');
  if (sourceIcon && targetIcon) {
    targetIcon.className = `search-engine-icon-svg ${sourceIcon.classList.contains('search-engine-icon-with-bg') ? 'search-engine-icon-with-bg' : ''}`;
    targetIcon.innerHTML = sourceIcon.innerHTML;
  }
}

function switchGroup(groupId) {
  const panels = qsa('.group-panel');
  const navs = qsa('.group-nav');
  if (!panels.length || !navs.length) return;
  const requestedGroupId = groupId || panels[0]?.dataset.groupId || null;
  const targetPanel = panels.find(panel => panel.dataset.groupId === requestedGroupId) || panels[0];
  activeGroupId = targetPanel?.dataset.groupId || null;
  panels.forEach(panel => panel.classList.toggle('is-active', panel.dataset.groupId === activeGroupId));
  navs.forEach(nav => nav.classList.toggle('is-active', nav.dataset.groupId === activeGroupId));
  storeActiveGroupId(activeGroupId);
}

function bindStaticActions() {
  qsa('[data-close]').forEach(btn => btn.addEventListener('click', closeAllModals));
  qs('[data-action="open-group-modal"]')?.addEventListener('click', () => {
    const form = qs('#group-form');
    form.reset();
    qs('[name=id]', form).value = '';
    openModal('#group-modal');
  });
  qs('[data-action="open-settings-modal"]')?.addEventListener('click', () => {
    openModal('#settings-modal');
  });
  qsa('[data-group-nav]').forEach(btn => btn.addEventListener('click', () => switchGroup(btn.dataset.groupId)));
  qsa('.bookmark-item-add').forEach(item => item.addEventListener('click', () => openBookmarkModal(item.dataset.addBookmarkToGroup || activeGroupId || '')));
  qs('[data-action="open-search-engine-modal"]')?.addEventListener('click', () => openSearchEngineModal());
  qs('#search-engine-btn')?.addEventListener('click', (event) => {
    event.stopPropagation();
    toggleSearchEngineDock();
  });
  qs('#search-engine-dock')?.addEventListener('click', event => event.stopPropagation());
  bindSearchEngineTiles();
}

function initSingleGroupView() {
  const firstGroup = qs('.group-panel');
  switchGroup(getStoredActiveGroupId() || firstGroup?.dataset.groupId || null);
}

function bindGroupContextMenu() {
  qsa('.group-nav').forEach(item => item.addEventListener('contextmenu', (event) => {
    event.preventDefault();
    currentContextGroupId = item.dataset.groupId;
    currentContextGroupName = item.dataset.groupName || '';
    const menu = qs('#group-context-menu');
    menu.style.left = `${event.clientX}px`;
    menu.style.top = `${event.clientY}px`;
    menu.classList.remove('hidden');
  }));

  qs('[data-group-context-action="edit"]')?.addEventListener('click', () => {
    if (!currentContextGroupId) return;
    const form = qs('#group-form');
    form.reset();
    qs('[name=id]', form).value = currentContextGroupId;
    qs('[name=name]', form).value = currentContextGroupName;
    openModal('#group-modal');
    hideContextMenus();
  });

  qs('[data-group-context-action="delete"]')?.addEventListener('click', async () => {
    if (!currentContextGroupId) return;
    if (!confirm('确认删除该分组？组内网站将移到未分组。')) return;
    try {
      await send(`/api/groups/${currentContextGroupId}`, 'DELETE');
      location.reload();
    } catch (error) {
      toast(error.message);
    }
  });
}

function bindBookmarkContextMenu() {
  qsa('.bookmark-item').forEach(item => item.addEventListener('contextmenu', (event) => {
    if (!item.dataset.id) return;
    event.preventDefault();
    currentContextBookmarkId = item.dataset.id;
    const menu = qs('#bookmark-context-menu');
    menu.style.left = `${event.clientX}px`;
    menu.style.top = `${event.clientY}px`;
    menu.classList.remove('hidden');
  }));

  document.addEventListener('click', hideContextMenus);
  document.addEventListener('click', closeSearchEngineDock);
  document.addEventListener('scroll', hideContextMenus, true);
  document.addEventListener('scroll', closeSearchEngineDock, true);

  qs('[data-context-action="edit"]')?.addEventListener('click', () => {
    const item = qs(`.bookmark-item[data-id="${currentContextBookmarkId}"]`);
    if (!item) return;
    openBookmarkModal(item.dataset.groupId || '', {
      id: currentContextBookmarkId,
      title: item.dataset.title || '',
      url: item.dataset.url || '',
      groupId: item.dataset.groupId || '',
      iconSrc: item.dataset.iconSrc || ''
    });
    hideContextMenus();
  });

  qs('[data-context-action="refresh"]')?.addEventListener('click', async () => {
    if (!currentContextBookmarkId) return;
    try {
      await send(`/api/bookmarks/${currentContextBookmarkId}/refresh-icon`, 'POST');
      location.reload();
    } catch (error) {
      toast(error.message);
    }
  });

  qs('[data-context-action="delete"]')?.addEventListener('click', async () => {
    if (!currentContextBookmarkId) return;
    if (!confirm('确认删除该网站？')) return;
    try {
      await send(`/api/bookmarks/${currentContextBookmarkId}`, 'DELETE');
      location.reload();
    } catch (error) {
      toast(error.message);
    }
  });
}

function bindWallpaperActions() {
  qsa('[data-activate-wallpaper]').forEach(btn => btn.addEventListener('click', async () => {
    try {
      await send(`/api/wallpapers/${btn.dataset.activateWallpaper}`, 'POST');
      location.reload();
    } catch (error) {
      toast(error.message);
    }
  }));

  qsa('[data-delete-wallpaper]').forEach(btn => btn.addEventListener('click', async () => {
    if (!confirm('确认删除该壁纸？')) return;
    try {
      await send(`/api/wallpapers/${btn.dataset.deleteWallpaper}`, 'DELETE');
      location.reload();
    } catch (error) {
      toast(error.message);
    }
  }));
}

function bindSearchEngineTiles() {
  qsa('.search-engine-tile[data-search-engine-id]').forEach(tile => {
    tile.addEventListener('click', async () => {
      closeSearchEngineDock();
      try {
        await send(`/api/search-engines/${tile.dataset.searchEngineId}/activate`, 'POST');
        setActiveSearchEngineTile(tile);
      } catch (error) {
        toast(error.message);
      }
    });
    tile.addEventListener('contextmenu', event => {
      event.preventDefault();
      currentContextSearchEngineId = tile.dataset.searchEngineId;
      currentContextSearchEngineName = tile.dataset.searchEngineName || '';
      currentContextSearchEngineTemplate = tile.dataset.searchEngineTemplate || '';
      closeSearchEngineDock();
      const menu = qs('#search-engine-context-menu');
      menu.style.left = `${event.clientX}px`;
      menu.style.top = `${event.clientY}px`;
      menu.classList.remove('hidden');
    });
  });

  qs('[data-search-engine-context-action="edit"]')?.addEventListener('click', () => {
    if (!currentContextSearchEngineId) return;
    openSearchEngineModal({
      id: currentContextSearchEngineId,
      name: currentContextSearchEngineName,
      template: currentContextSearchEngineTemplate
    });
    hideContextMenus();
  });

  qs('[data-search-engine-context-action="delete"]')?.addEventListener('click', async () => {
    if (!currentContextSearchEngineId) return;
    if (!confirm('确认删除该搜索引擎？')) return;
    try {
      await send(`/api/search-engines/${currentContextSearchEngineId}`, 'DELETE');
      location.reload();
    } catch (error) {
      toast(error.message);
    }
  });
}

function closestBookmarkForDrop(items, x, y) {
  return items.reduce((closest, item) => {
    const rect = item.getBoundingClientRect();
    const centerX = rect.left + rect.width / 2;
    const centerY = rect.top + rect.height / 2;
    const distance = Math.hypot(x - centerX, y - centerY);
    if (!closest || distance < closest.distance) {
      return { item, distance, rect };
    }
    return closest;
  }, null);
}

async function saveBookmarkOrder(grid) {
  const ids = qsa('.bookmark-item[data-id]', grid).map(item => item.dataset.id).filter(Boolean);
  if (!ids.length) return;
  const fd = new FormData();
  fd.set('ids', ids.join(','));
  await send('/api/bookmarks/reorder', 'PUT', fd);
}

async function saveGroupOrder() {
  const ids = qsa('.group-nav').map(item => item.dataset.groupId).filter(Boolean);
  if (!ids.length) return;
  const fd = new FormData();
  fd.set('ids', ids.join(','));
  await send('/api/groups/reorder', 'PUT', fd);
}

function bindBookmarkDragSort() {
  qsa('.bookmark-grid').forEach(grid => {
    let dragged = null;
    let orderChanged = false;

    qsa('.bookmark-item[data-id]', grid).forEach(item => {
      item.draggable = true;
      item.addEventListener('dragstart', event => {
        dragged = item;
        orderChanged = false;
        item.classList.add('is-dragging');
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData('text/plain', item.dataset.id || '');
      });
      item.addEventListener('dragend', async () => {
        item.classList.remove('is-dragging');
        grid.classList.remove('is-drag-over');
        if (orderChanged) {
          try {
            await saveBookmarkOrder(grid);
          } catch (error) {
            toast(error.message);
            location.reload();
          }
        }
        dragged = null;
        orderChanged = false;
      });
    });

    grid.addEventListener('dragover', event => {
      if (!dragged || !grid.contains(dragged)) return;
      event.preventDefault();
      grid.classList.add('is-drag-over');
      const items = qsa('.bookmark-item[data-id]:not(.is-dragging)', grid);
      const closest = closestBookmarkForDrop(items, event.clientX, event.clientY);
      if (!closest) return;
      const insertAfter = event.clientX > closest.rect.left + closest.rect.width / 2 || event.clientY > closest.rect.top + closest.rect.height * 0.72;
      const reference = insertAfter ? closest.item.nextElementSibling : closest.item;
      if (reference !== dragged && reference !== dragged.nextElementSibling) {
        grid.insertBefore(dragged, reference);
        orderChanged = true;
      }
    });

    grid.addEventListener('dragleave', event => {
      if (!grid.contains(event.relatedTarget)) {
        grid.classList.remove('is-drag-over');
      }
    });
  });
}

function bindGroupDragSort() {
  const container = qs('.sidebar-top');
  if (!container) return;

  let dragged = null;
  let orderChanged = false;

  function groupDropReference(clientY) {
    const items = qsa('.group-nav:not(.is-dragging)', container);
    for (const item of items) {
      const rect = item.getBoundingClientRect();
      if (clientY < rect.top + rect.height / 2) {
        return item;
      }
    }
    return qs('.nav-entry-add', container);
  }

  qsa('.group-nav').forEach(item => {
    item.draggable = true;

    item.addEventListener('dragstart', event => {
      dragged = item;
      orderChanged = false;
      item.classList.add('is-dragging');
      container.classList.add('is-group-dragging');
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', item.dataset.groupId || '');
    });

    item.addEventListener('dragend', async () => {
      item.classList.remove('is-dragging');
      container.classList.remove('is-group-dragging');
      if (orderChanged) {
        try {
          await saveGroupOrder();
        } catch (error) {
          toast(error.message);
          location.reload();
        }
      }
      dragged = null;
      orderChanged = false;
    });
  });

  container.addEventListener('dragover', event => {
    if (!dragged || !container.contains(dragged)) return;
    event.preventDefault();
    const reference = groupDropReference(event.clientY);
    if (reference !== dragged && reference !== dragged.nextElementSibling) {
      container.insertBefore(dragged, reference);
      orderChanged = true;
    }
  });

  container.addEventListener('dragleave', event => {
    if (!container.contains(event.relatedTarget)) {
      container.classList.remove('is-group-dragging');
    }
  });
}

qs('#group-form')?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const fd = new FormData(form);
  const id = fd.get('id');
  try {
    setSubmitting(form, true);
    await send(id ? `/api/groups/${id}` : '/api/groups', id ? 'PUT' : 'POST', fd);
    closeAllModals();
    location.reload();
  } catch (error) { toast(error.message); } finally { setSubmitting(form, false); }
});

qs('#bookmark-form')?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const fd = new FormData(form);
  const id = fd.get('id');
  if (!(fd.get('title') || '').toString().trim() || !(fd.get('url') || '').toString().trim()) {
    toast('请填写网站名称和网址');
    return;
  }
  try {
    setSubmitting(form, true);
    await send(id ? `/api/bookmarks/${id}` : '/api/bookmarks', id ? 'PUT' : 'POST', fd);
    closeAllModals();
    location.reload();
  } catch (error) { toast(error.message); } finally { setSubmitting(form, false); }
});

qs('#bookmark-form [name=icon]')?.addEventListener('change', (event) => {
  const file = event.currentTarget.files?.[0];
  if (!file) {
    const title = qs('#bookmark-form [name=title]')?.value || '';
    setBookmarkIconPreview('', title);
    return;
  }
  const objectURL = URL.createObjectURL(file);
  setBookmarkIconPreview(objectURL, file.name);
  bookmarkIconPreviewURL = objectURL;
});

qs('#search-engine-form')?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const fd = new FormData(form);
  const id = fd.get('id');
  if (!(fd.get('name') || '').toString().trim() || !(fd.get('url_template') || '').toString().trim()) {
    toast('请填写搜索名称和网址模板');
    return;
  }
  try {
    setSubmitting(form, true);
    await send(id ? `/api/search-engines/${id}` : '/api/search-engines', id ? 'PUT' : 'POST', fd);
    closeAllModals();
    location.reload();
  } catch (error) { toast(error.message); } finally { setSubmitting(form, false); }
});

qs('#wallpaper-upload-btn')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  const input = qs('#wallpaper-form [name=wallpaper]');
  const file = input?.files?.[0];
  if (!file) {
    toast('请选择壁纸文件');
    return;
  }
  const fd = new FormData();
  fd.set('wallpaper', file);
  try {
    setButtonSubmitting(button, true);
    await send('/api/wallpapers', 'POST', fd);
    closeAllModals();
    location.reload();
  } catch (error) { toast(error.message); } finally { setButtonSubmitting(button, false); }
});

qs('#settings-form')?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const fd = new FormData(form);
  try {
    setSubmitting(form, true);
    await send('/api/settings', 'PUT', fd);
    closeAllModals();
    location.reload();
  } catch (error) { toast(error.message); } finally { setSubmitting(form, false); }
});

qs('#search-form')?.addEventListener('submit', (event) => {
  event.preventDefault();
  const input = qs('#search-input');
  const keyword = (input?.value || '').trim();
  if (!keyword) return;
  const searchEngine = qs('.search-engine-tile.is-active')?.dataset.searchEngineTemplate || event.currentTarget.dataset.defaultSearchEngine || '';
  if (!searchEngine) return;
  window.open(searchEngine.replace('%s', encodeURIComponent(keyword)), '_blank', 'noopener,noreferrer');
});

initSingleGroupView();
bindStaticActions();
bindGroupContextMenu();
bindBookmarkContextMenu();
bindBookmarkDragSort();
bindGroupDragSort();
bindWallpaperActions();
