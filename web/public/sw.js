// norvels service worker：离线缓存静态资源、APP shell 与最近阅读章节。
// 无构建期注入，Vite 只需把 public/sw.js 原样复制到 dist 根。
const STATIC_CACHE = "norvels-static-v1";
const SHELL_CACHE = "norvels-shell-v1";
const CHAPTER_CACHE = "norvels-chapters-v1";
const MAX_CHAPTERS = 24;
const APP_SHELL = ["/", "/index.html", "/manifest.webmanifest", "/favicon.svg"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(SHELL_CACHE).then((cache) => cache.addAll(APP_SHELL)).catch(() => undefined),
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  const keep = new Set([STATIC_CACHE, SHELL_CACHE, CHAPTER_CACHE]);
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(keys.filter((key) => !keep.has(key)).map((key) => caches.delete(key)));
      await self.clients.claim();
    })(),
  );
});

function isStaticAsset(pathname) {
  if (pathname.startsWith("/assets/")) return true;
  return /\.(js|mjs|css|map|json|png|jpg|jpeg|gif|svg|ico|webp|avif|woff2?|ttf|otf|eot|txt|xml|webmanifest|wasm)$/i.test(pathname);
}

function isChapterApi(url) {
  return /^\/api\/books\/[^/]+\/chapters\/[^/]+$/.test(url.pathname);
}

async function trimCache(cache, maxEntries) {
  const keys = await cache.keys();
  while (keys.length >= maxEntries) {
    const key = keys.shift();
    if (key) await cache.delete(key);
  }
}

async function cacheFirst(request, cacheName) {
  const cache = await caches.open(cacheName);
  const cached = await cache.match(request);
  if (cached) return cached;
  try {
    const response = await fetch(request);
    if (response && response.ok) {
      await cache.put(request, response.clone());
    }
    return response;
  } catch (error) {
    if (cached) return cached;
    throw error;
  }
}

async function networkFirstShell(request) {
  const cache = await caches.open(SHELL_CACHE);
  try {
    const response = await fetch(request);
    if (response && response.ok) {
      await cache.put("/index.html", response.clone());
      await cache.put(request, response.clone());
    }
    return response;
  } catch (error) {
    const cached =
      (await cache.match(request)) ||
      (await cache.match("/index.html")) ||
      (await cache.match("/"));
    if (cached) return cached;
    return new Response("离线阅读：当前页面尚未缓存", {
      status: 503,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }
}

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;
  let url;
  try {
    url = new URL(request.url);
  } catch {
    return;
  }
  if (url.origin !== self.location.origin) return;

  if (isChapterApi(url)) {
    event.respondWith(
      (async () => {
        const cache = await caches.open(CHAPTER_CACHE);
        const cached = await cache.match(request);
        if (cached) {
          // 后台刷新，失败静默。
          fetch(request)
            .then((response) => {
              if (response && response.ok) {
                trimCache(cache, MAX_CHAPTERS).then(() => cache.put(request, response));
              }
            })
            .catch(() => undefined);
          return cached;
        }
        try {
          const response = await fetch(request);
          if (response && response.ok) {
            await trimCache(cache, MAX_CHAPTERS);
            await cache.put(request, response.clone());
          }
          return response;
        } catch (error) {
          return new Response(JSON.stringify({ error: { code: "OFFLINE", message: "离线阅读：章节尚未缓存" } }), {
            status: 503,
            headers: { "Content-Type": "application/json; charset=utf-8" },
          });
        }
      })(),
    );
    return;
  }

  if (request.mode === "navigate") {
    event.respondWith(networkFirstShell(request));
    return;
  }

  if (isStaticAsset(url.pathname)) {
    event.respondWith(cacheFirst(request, STATIC_CACHE));
  }
});
