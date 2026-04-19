const { app, BrowserWindow, ipcMain, shell } = require('electron');
const path = require('path');
const fs = require('fs');
const os = require('os');

const DESKTOP = path.join(os.homedir(), 'Desktop');
const IMAGE_EXTS = new Set(['.png', '.jpg', '.jpeg', '.webp', '.gif']);

function getDesktopImages() {
  try {
    const entries = fs.readdirSync(DESKTOP);
    const images = [];
    for (const name of entries) {
      if (name.startsWith('.')) continue;
      const ext = path.extname(name).toLowerCase();
      if (!IMAGE_EXTS.has(ext)) continue;
      const full = path.join(DESKTOP, name);
      try {
        const stat = fs.statSync(full);
        images.push({ path: full, name, mtime: stat.mtimeMs, status: 'new' });
      } catch {}
    }
    images.sort((a, b) => b.mtime - a.mtime);
    return images;
  } catch (e) {
    console.error('getDesktopImages error:', e);
    return [];
  }
}

function createWindow() {
  const win = new BrowserWindow({
    width: 1200,
    height: 800,
    title: 'xmuggle',
    backgroundColor: '#1a1a2e',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  win.loadFile(path.join(__dirname, 'renderer', 'index.html'));

  // Watch Desktop for changes
  try {
    fs.watch(DESKTOP, () => {
      try {
        win.webContents.send('images-updated', getDesktopImages());
      } catch {}
    });
  } catch {}
}

app.whenReady().then(() => {
  const api = require('./api');

  ipcMain.handle('get-images', () => getDesktopImages());
  ipcMain.handle('delete-image', (_, imgPath) => {
    try { fs.unlinkSync(imgPath); } catch {}
    return getDesktopImages();
  });
  ipcMain.handle('has-api-key', () => api.hasApiKey());
  ipcMain.handle('set-api-key', (_, key) => { api.setApiKey(key); return true; });
  ipcMain.handle('reset-api-key', () => { api.resetApiKey(); return true; });
  ipcMain.handle('open-external', (_, url) => shell.openExternal(url));
  ipcMain.handle('send-to-api', async (_, imagePaths, repo, message) => {
    return api.analyzeAndFix({ imagePaths, repo, message });
  });

  createWindow();
});

app.on('window-all-closed', () => app.quit());
