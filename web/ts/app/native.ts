// Приложение для Android (Capacitor): если сайт открыт внутри него, трек
// пишется нативным фоновым источником — и при выключенном экране, с
// уведомлением. В браузере этот файл ничего не делает.
import { Fix, markBackgroundCapable, setPositionSource } from "./track.js";

interface NativeLocation { latitude: number; longitude: number; altitude: number | null; accuracy: number; time: number | null }
interface NativeError { code?: string; message: string }
interface BackgroundGeolocation {
  addWatcher(options: {
    backgroundMessage?: string; backgroundTitle?: string; requestPermissions?: boolean; stale?: boolean; distanceFilter?: number;
  }, callback: (location?: NativeLocation, error?: NativeError) => void): Promise<string>;
  removeWatcher(options: { id: string }): Promise<void>;
  openSettings(): Promise<void>;
}
interface CapacitorGlobal { isNativePlatform?: () => boolean; Plugins?: { BackgroundGeolocation?: BackgroundGeolocation } }

export function setupNative() {
  const cap = (window as unknown as { Capacitor?: CapacitorGlobal }).Capacitor;
  const bg = cap?.isNativePlatform?.() ? cap.Plugins?.BackgroundGeolocation : undefined;
  if (!bg) return;
  markBackgroundCapable();
  setPositionSource((onFix, onError) => {
    let id: string | null = null;
    let stopped = false;
    bg.addWatcher({
      backgroundTitle: "Идёт запись трека",
      backgroundMessage: "Нажмите, чтобы открыть приложение",
      requestPermissions: true,
      stale: false,
      distanceFilter: 5,
    }, (loc, err) => {
      if (err) {
        onError(err.code === "NOT_AUTHORIZED" ? "Нет доступа к геолокации: разрешите его приложению в настройках телефона." : err.message);
        return;
      }
      if (loc) {
        const f: Fix = { lat: loc.latitude, lon: loc.longitude, ele: loc.altitude, acc: loc.accuracy, t: loc.time ?? Date.now() };
        onFix(f);
      }
    }).then((wid) => { if (stopped) bg.removeWatcher({ id: wid }).catch(() => {}); else id = wid; }, (e: Error) => onError(e.message));
    return () => { stopped = true; if (id) bg.removeWatcher({ id }).catch(() => {}); };
  });
}
