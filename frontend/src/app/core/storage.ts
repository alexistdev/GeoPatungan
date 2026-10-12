/** Akses localStorage yang aman: bisa melempar error (mode privat, data situs diblokir). */
export const safeStorage = {
  get(key: string): string | null {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  },
  set(key: string, value: string): void {
    try {
      localStorage.setItem(key, value);
    } catch {
      /* diabaikan: aplikasi tetap berjalan tanpa penyimpanan */
    }
  },
  remove(key: string): void {
    try {
      localStorage.removeItem(key);
    } catch {
      /* diabaikan */
    }
  },
};
