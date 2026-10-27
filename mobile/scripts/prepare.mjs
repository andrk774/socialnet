// Готовит Android-проект Capacitor: конфиг с адресом сайта, проект android/
// (создаётся при первом запуске), разрешения на геолокацию в фоне, подпись
// и номер версии. Запуск: APP_URL=https://ваш-домен node scripts/prepare.mjs
//
// Приложение — оболочка сайта: интерфейс грузится с APP_URL, поэтому
// обновления сайта приходят без новой версии APK. Своё в приложении —
// фоновая запись трека (плагин background-geolocation): GPS пишет и при
// выключенном экране, с уведомлением «Идёт запись трека».
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const env = process.env;
const appURL = (env.APP_URL || "").replace(/\/+$/, "");
if (!/^https:\/\/[^/]+$/.test(appURL)) {
  console.error("APP_URL: адрес сайта вида https://example.ru (без пути)");
  process.exit(2);
}
const host = new URL(appURL).host;
const appId = env.APP_ID || "ru.socialnet.app";
const appName = env.APP_NAME || "socialnet";
const versionCode = Number(env.VERSION_CODE || 1);
const versionName = env.VERSION_NAME || `1.0.${versionCode}`;

const config = {
  appId,
  appName,
  webDir: "www",
  server: { url: appURL, androidScheme: "https", allowNavigation: [host], errorPath: "index.html" },
  android: { allowMixedContent: false },
};
writeFileSync(join(root, "capacitor.config.json"), JSON.stringify(config, null, 2) + "\n");

const run = (...args) => execFileSync("npx", ["--no-install", ...args], { cwd: root, stdio: "inherit" });
if (!existsSync(join(root, "android"))) run("cap", "add", "android");

const edit = (rel, fn) => {
  const p = join(root, rel);
  const before = readFileSync(p, "utf8");
  const after = fn(before);
  if (after !== before) writeFileSync(p, after);
};

// разрешения: точная геолокация, в фоне — через foreground service плагина
const perms = [
  "android.permission.ACCESS_COARSE_LOCATION",
  "android.permission.ACCESS_FINE_LOCATION",
  "android.permission.FOREGROUND_SERVICE",
  "android.permission.FOREGROUND_SERVICE_LOCATION",
  "android.permission.POST_NOTIFICATIONS",
];
edit("android/app/src/main/AndroidManifest.xml", (xml) => {
  for (const p of perms) {
    if (!xml.includes(`"${p}"`)) xml = xml.replace("</manifest>", `    <uses-permission android:name="${p}" />\n</manifest>`);
  }
  if (!xml.includes("android.hardware.location.gps")) {
    xml = xml.replace("</manifest>", `    <uses-feature android:name="android.hardware.location.gps" android:required="false" />\n</manifest>`);
  }
  return xml;
});

edit("android/app/src/main/res/values/strings.xml", (xml) => {
  const add = (name, value) => {
    if (!xml.includes(`name="${name}"`)) xml = xml.replace("</resources>", `    <string name="${name}">${value}</string>\n</resources>`);
  };
  add("capacitor_background_geolocation_notification_channel_name", "Запись трека");
  return xml;
});

// номер версии и подпись релиза из переменных окружения (секреты CI)
edit("android/app/build.gradle", (g) => {
  g = g.replace(/versionCode \d+/, `versionCode ${versionCode}`).replace(/versionName "[^"]*"/, `versionName "${versionName}"`);
  if (!g.includes("ANDROID_KEYSTORE")) {
    g += `
// подпись релиза: ANDROID_KEYSTORE (путь к .jks), ANDROID_KEYSTORE_PASSWORD,
// ANDROID_KEY_ALIAS, ANDROID_KEY_PASSWORD — добавлено scripts/prepare.mjs
android {
    signingConfigs {
        release {
            if (System.getenv("ANDROID_KEYSTORE")) {
                storeFile file(System.getenv("ANDROID_KEYSTORE"))
                storePassword System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias System.getenv("ANDROID_KEY_ALIAS")
                keyPassword System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
    }
    buildTypes {
        release {
            if (System.getenv("ANDROID_KEYSTORE")) {
                signingConfig signingConfigs.release
            }
        }
    }
}
`;
  }
  return g;
});

run("cap", "sync", "android");
console.log(`android/ готов: ${appName} (${appId}) ${versionName}, сайт ${appURL}`);
