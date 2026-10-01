declare global {
  interface Window {
    dataLayer?: unknown[];
    gtag?: (...args: unknown[]) => void;
    ym?: (id: number | string, method: string, ...params: unknown[]) => void;
    yandexCounterId?: string | number;
  }
}

const YM_ID = import.meta.env.VITE_YM_ID || "113253415";
const YM_WEBVISOR = import.meta.env.VITE_YM_WEBVISOR !== "false";
const GA_ID = import.meta.env.VITE_GA_ID;

let isInitialized = false;

/**
 * Инициализирует Яндекс.Метрику и Google Analytics 4 при наличии переменных окружения
 */
export function initAnalytics(): void {
  if (isInitialized || typeof window === "undefined") return;
  isInitialized = true;

  // Инициализация Яндекс.Метрики
  if (YM_ID) {
    try {
      const ymIdNum = Number(YM_ID) || YM_ID;
      window.yandexCounterId = ymIdNum;

      const ymWin = window as unknown as Record<string, unknown>;
      ymWin.ym =
        ymWin.ym ||
        function (...args: unknown[]) {
          ((ymWin.ym as unknown as { a?: unknown[] }).a =
            (ymWin.ym as unknown as { a?: unknown[] }).a || []).push(args);
        };
      (ymWin.ym as unknown as { l: number }).l = 1 * new Date().getTime();

      const scriptSrc = `https://mc.yandex.ru/metrika/tag.js?id=${encodeURIComponent(String(ymIdNum))}`;
      let scriptExists = false;
      for (let j = 0; j < document.scripts.length; j++) {
        if (document.scripts[j].src === scriptSrc) {
          scriptExists = true;
          break;
        }
      }

      if (!scriptExists) {
        const k = document.createElement("script");
        const a = document.getElementsByTagName("script")[0];
        k.async = true;
        k.src = scriptSrc;
        a?.parentNode?.insertBefore(k, a);
      }

      window.ym?.(ymIdNum, "init", {
        ssr: true,
        webvisor: YM_WEBVISOR,
        clickmap: true,
        ecommerce: "dataLayer",
        referrer: document.referrer,
        url: window.location.href,
        accurateTrackBounce: true,
        trackLinks: true,
      });
    } catch {
      // Игнорируем ошибки инициализации (например, блокировщики рекламы)
    }
  }

  // Инициализация Google Analytics 4 (gtag.js)
  if (GA_ID) {
    try {
      window.dataLayer = window.dataLayer || [];
      window.gtag = function () {
        // eslint-disable-next-line prefer-rest-params
        window.dataLayer?.push(arguments);
      };
      window.gtag("js", new Date());
      window.gtag("config", GA_ID, {
        send_page_view: false, // ручной трекинг через trackPageView для SPA
      });

      const script = document.createElement("script");
      script.async = true;
      script.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(GA_ID)}`;
      document.head.appendChild(script);
    } catch {
      // Игнорируем ошибки инициализации
    }
  }
}

/**
 * Отправка виртуального просмотра страницы (SPA-переход)
 */
export function trackPageView(path: string, title?: string): void {
  const currentTitle = title || document.title;
  const currentUrl = window.location.origin + path;

  // Яндекс.Метрика: вызов hit
  if (YM_ID && typeof window.ym === "function") {
    try {
      const ymIdNum = Number(YM_ID) || YM_ID;
      window.ym(ymIdNum, "hit", currentUrl, {
        title: currentTitle,
        referer: document.referrer,
      });
    } catch {
      // noop
    }
  }

  // Google Analytics 4: событие page_view
  if (GA_ID && typeof window.gtag === "function") {
    try {
      window.gtag("event", "page_view", {
        page_path: path,
        page_title: currentTitle,
        page_location: currentUrl,
      });
    } catch {
      // noop
    }
  }
}

/**
 * Отправка произвольного события / цели
 */
export function trackEvent(
  eventName: string,
  params: Record<string, unknown> = {},
): void {
  // Яндекс.Метрика: reachGoal и params
  if (YM_ID && typeof window.ym === "function") {
    try {
      const ymIdNum = Number(YM_ID) || YM_ID;
      window.ym(ymIdNum, "reachGoal", eventName, params);
      window.ym(ymIdNum, "params", { [eventName]: params });
    } catch {
      // noop
    }
  }

  // Google Analytics 4: event
  if (GA_ID && typeof window.gtag === "function") {
    try {
      window.gtag("event", eventName, params);
    } catch {
      // noop
    }
  }
}

/**
 * Специализированные события аналитики Avari Domains
 */

export function trackDomainCheckStart(
  input: string,
  source: "input" | "suggestion" | "history",
): void {
  trackEvent("domain_check_started", {
    domain_query: input.toLowerCase(),
    source,
  });
}

export function trackDomainCheckSuccess(
  domain: string,
  status: string,
  cached: boolean,
  durationMs?: number,
): void {
  trackEvent("domain_check_completed", {
    domain: domain.toLowerCase(),
    status,
    cached,
    duration_ms: durationMs,
  });
}

export function trackDomainCheckError(input: string, errorCode: string): void {
  trackEvent("domain_check_failed", {
    input: input.toLowerCase(),
    error_code: errorCode,
  });
}

export function trackRegistrarClick(
  registrarName: string,
  domain: string,
  priceReg?: string,
  priceRen?: string,
): void {
  trackEvent("registrar_click", {
    registrar: registrarName,
    domain: domain.toLowerCase(),
    price_register: priceReg,
    price_renew: priceRen,
  });
}

export function trackSuggestionClick(domain: string): void {
  trackEvent("suggestion_click", {
    domain: domain.toLowerCase(),
  });
}

export function trackSuggestionRefresh(): void {
  trackEvent("suggestion_refresh");
}

export function trackLanguageSwitch(newLang: "ru" | "en"): void {
  trackEvent("language_switched", {
    language: newLang,
  });
}

export function trackHistoryView(page: number): void {
  trackEvent("history_view", {
    history_page: page,
  });
}

export function trackHistoryItemClick(domain: string): void {
  trackEvent("history_item_click", {
    domain: domain.toLowerCase(),
  });
}

export function trackScrollToTop(): void {
  trackEvent("scroll_to_top");
}
