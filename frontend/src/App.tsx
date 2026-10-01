import { useState, useEffect, useRef, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import type { Result, History, Stats, Finding, ApiError } from "./types";
import {
  initAnalytics,
  trackPageView,
  trackDomainCheckStart,
  trackDomainCheckSuccess,
  trackDomainCheckError,
  trackRegistrarClick,
  trackSuggestionClick,
  trackSuggestionRefresh,
  trackLanguageSwitch,
  trackHistoryView,
  trackHistoryItemClick,
  trackScrollToTop,
} from "./analytics";

async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const r = await fetch("/api" + path, {
    credentials: "same-origin",
    ...options,
  });
  const data = await r.json();
  if (!r.ok) throw data as ApiError;
  return data as T;
}
const siteItems: [string, string, string][] = [
  ["a", "A", "IPv4 адрес"],
  ["aaaa", "AAAA", "IPv6 адрес"],
  ["cname", "CNAME", "Псевдоним"],
  ["ns", "NS", "Серверы имён"],
  ["delegation", "Делегирование", "Согласованность NS"],
  ["tls", "TLS", "Сертификат и доверие"],
  ["http", "HTTP", "Ответ без шифрования"],
  ["https", "HTTPS", "Ответ с шифрованием"],
];
const mailItems: [string, string, string][] = [
  ["mx", "MX", "Приём почты"],
  ["spf", "SPF", "Разрешённые отправители"],
  ["dkim", "DKIM", "Подпись сообщений"],
  ["dmarc", "DMARC", "Политика домена"],
];
const labels: Record<string, string> = {
  registered: "Зарегистрирован",
  unregistered: "Вероятно свободен",
  available: "Свободен",
  unknown: "Статус неизвестен",
};
const errorMessages: Record<string, [string, string]> = {
  input_required: ["Введите домен или URL.", "Enter a domain or URL."],
  input_too_long: [
    "Длина домена или URL не должна превышать 2048 символов.",
    "The domain or URL must be 2048 characters or fewer.",
  ],
  invalid_url: [
    "Введите домен или HTTP/HTTPS-ссылку без логина и пароля.",
    "Enter a domain or an HTTP/HTTPS URL without login details.",
  ],
  unsupported_port: [
    "Проверка нестандартных портов не поддерживается.",
    "Custom ports are not supported.",
  ],
  ip_not_supported: [
    "Введите доменное имя вместо IP-адреса.",
    "Enter a domain name instead of an IP address.",
  ],
  invalid_domain: [
    "Проверьте написание доменного имени.",
    "Check that the domain name is valid.",
  ],
  registrable_domain_not_found: [
    "Не удалось определить домен для проверки регистрации.",
    "Could not determine the registrable domain.",
  ],
  invalid_input: [
    "Проверьте доменное имя или URL и попробуйте снова.",
    "Check the domain name or URL and try again.",
  ],
  invalid_json: [
    "Не удалось обработать запрос. Обновите страницу и попробуйте снова.",
    "The request could not be processed. Refresh the page and try again.",
  ],
  rate_limited: [
    "Слишком много запросов. Подождите несколько секунд и повторите попытку.",
    "Too many requests. Wait a few seconds and try again.",
  ],
  busy: [
    "Сервис временно занят. Попробуйте ещё раз чуть позже.",
    "The service is temporarily busy. Please try again shortly.",
  ],
  invalid_page: [
    "Не удалось открыть эту страницу истории.",
    "This history page could not be opened.",
  ],
  storage_error: [
    "Не удалось загрузить данные. Попробуйте обновить страницу.",
    "Could not load the data. Try refreshing the page.",
  ],
  not_found: [
    "Запрошенный результат не найден.",
    "The requested result was not found.",
  ],
  network_error: [
    "Не удалось связаться с сервисом. Проверьте соединение и повторите попытку.",
    "Could not reach the service. Check your connection and try again.",
  ],
  request_failed: [
    "Не удалось выполнить запрос. Попробуйте ещё раз.",
    "The request could not be completed. Please try again.",
  ],
};
function userError(error: unknown, en: boolean): string {
  const code = (error as ApiError | undefined)?.code;
  const fallback = code ? "request_failed" : "network_error";
  return (
    errorMessages[code || ""]?.[en ? 1 : 0] ||
    errorMessages[fallback][en ? 1 : 0]
  );
}

const POPULAR_DOMAINS = [
  "google.com",
  "yandex.ru",
  "github.com",
  "telegram.org",
  "habr.com",
  "vk.com",
  "wikipedia.org",
  "reddit.com",
  "apple.com",
  "microsoft.com",
  "amazon.com",
  "netflix.com",
  "youtube.com",
  "cloudflare.com",
  "gitlab.com",
  "digitalocean.com",
  "rbc.ru",
  "ozon.ru",
  "wildberries.ru",
  "t-bank.ru",
];

const WORD_ADJECTIVES = [
  "crazy",
  "cyber",
  "super",
  "hyper",
  "neon",
  "pixel",
  "space",
  "rapid",
  "silent",
  "quantum",
  "turbo",
  "epic",
  "cosmic",
  "stellar",
  "shadow",
  "magic",
  "swift",
  "lucky",
  "nova",
  "prime",
  "vivid",
  "solar",
  "crypto",
  "retro",
];

const WORD_NOUNS = [
  "hulk",
  "fox",
  "dragon",
  "rocket",
  "ninja",
  "storm",
  "pilot",
  "spark",
  "tiger",
  "falcon",
  "nexus",
  "matrix",
  "vector",
  "orbit",
  "craft",
  "pulse",
  "haven",
  "beacon",
  "forge",
  "drift",
  "quest",
  "wave",
  "wolf",
  "shield",
];

const POPULAR_TLDS = [
  ".com",
  ".ru",
  ".org",
  ".net",
  ".io",
  ".dev",
  ".ai",
  ".co",
  ".app",
  ".me",
  ".info",
  ".xyz",
  ".tech",
  ".online",
  ".store",
  ".site",
  ".space",
  ".club",
  ".pro",
  ".biz",
];

function generateRandomDomain(): string {
  const adj =
    WORD_ADJECTIVES[Math.floor(Math.random() * WORD_ADJECTIVES.length)];
  const noun = WORD_NOUNS[Math.floor(Math.random() * WORD_NOUNS.length)];
  const tld = POPULAR_TLDS[Math.floor(Math.random() * POPULAR_TLDS.length)];
  return `${adj}-${noun}${tld}`;
}

function getRandomSuggestions(): string[] {
  const shuffledPopular = [...POPULAR_DOMAINS].sort(() => 0.5 - Math.random());
  const popularSample = shuffledPopular.slice(0, 2);
  const genSample = [generateRandomDomain(), generateRandomDomain()];
  return [...popularSample, ...genSample];
}
function Logo() {
  return (
    <svg
      viewBox="0 0 56 56"
      className="logo"
      role="img"
      aria-label="Avari Domains"
    >
      <defs>
        <linearGradient id="blade" x1="0" x2="1" y1="1" y2="0">
          <stop stopColor="#ff7a32" />
          <stop offset="1" stopColor="#86e8ff" />
        </linearGradient>
      </defs>
      <path
        d="M6 48 28 5l22 43-11-5-11-23-11 23Z"
        fill="none"
        stroke="url(#blade)"
        strokeWidth="3"
        strokeLinejoin="round"
      />
      <path d="M15 37h26M22 48l6-10 6 10" stroke="#8ee7ff" strokeWidth="2" />
      <path d="m5 49 10-3M51 49l-10-3" stroke="#ff8b44" strokeWidth="2" />
    </svg>
  );
}
function statusText(s: string) {
  return (
    (
      {
        ok: "Работает",
        warning: "Внимание",
        missing: "Не найдено",
        neutral: "Необязательно",
        unknown: "Не удалось проверить",
      } as Record<string, string>
    )[s] || s
  );
}
function Card({
  title,
  subtitle,
  data,
  index,
  reduced,
  en,
}: {
  title: string;
  subtitle: string;
  data?: Finding;
  index: number;
  reduced: boolean;
  en: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <motion.article
      initial={reduced ? false : { opacity: 0, y: 18 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: index * 0.045 }}
      className={"check-card " + (data?.status || "unknown")}
    >
      <button
        className="card-head"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
      >
        <span className="card-icon">
          {data?.status === "ok"
            ? "✓"
            : data?.status === "warning"
              ? "!"
              : data?.status === "missing"
                ? "–"
                : data?.status === "neutral"
                  ? "·"
                  : "?"}
        </span>
        <span className="card-title">
          <strong>{title}</strong>
          <small>{subtitle}</small>
        </span>
        <span className="card-right">
          <span className="status">
            {en
              ? (
                  {
                    ok: "Operational",
                    warning: "Warning",
                    missing: "Not found",
                    neutral: "Optional",
                    unknown: "Check failed",
                  } as Record<string, string>
                )[data?.status || "unknown"]
              : statusText(data?.status || "unknown")}
          </span>
          <span className="chevron">⌄</span>
        </span>
      </button>
      <p>{data?.summary || (en ? "Waiting for data" : "Ожидаем данные")}</p>
      <AnimatePresence>
        {open && (
          <motion.div
            className="card-details"
            initial={reduced ? false : { height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
          >
            <div>
              {data?.values?.map((v, i) => (
                <code key={i}>{v}</code>
              ))}
              {data?.detail && <span>{data.detail}</span>}
              <small>
                {en ? "Received: " : "Получено: "}
                {data?.checkedAt
                  ? new Date(data.checkedAt).toLocaleString(
                      en ? "en-US" : "ru-RU",
                    )
                  : "—"}
              </small>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.article>
  );
}
function Results({
  result,
  onRepeat,
  reduced,
  en,
}: {
  result: Result;
  onRepeat: (s: string) => void;
  reduced: boolean;
  en: boolean;
}) {
  const [tab, setTab] = useState<
    "site" | "mail" | "preview" | "robots" | "sitemap"
  >("site");
  const reg = result.registration.status;
  const preview = result.preview || {
    title: result.hostname,
    description: en
      ? "No preview description was found"
      : "Описание для предпросмотра не задано",
    image: "",
    siteName: result.hostname,
    url: "https://" + result.hostname + "/",
    status: "unknown",
    summary: en ? "Metadata unavailable" : "Метаданные недоступны",
  };
  const crawlFinding = tab === "robots" ? result.robots : result.sitemap;
  const items = (tab === "site" ? siteItems : mailItems).map(
    ([key, name, sub]) =>
      [
        key,
        en
          ? (
              {
                a: "IPv4 address",
                aaaa: "IPv6 address",
                cname: "Alias",
                ns: "Name servers",
                delegation: "NS consistency",
                tls: "Certificate and trust",
                http: "Unencrypted response",
                https: "Encrypted response",
                mx: "Mail reception",
                spf: "Authorized senders",
                dkim: "Message signing",
                dmarc: "Domain policy",
              } as Record<string, string>
            )[key]
          : name,
        en
          ? (
              {
                a: "IPv4 address",
                aaaa: "IPv6 address",
                cname: "Alias",
                ns: "Name servers",
                delegation: "NS consistency",
                tls: "Certificate and trust",
                http: "Unencrypted response",
                https: "Encrypted response",
                mx: "Mail reception",
                spf: "Authorized senders",
                dkim: "Message signing",
                dmarc: "Domain policy",
              } as Record<string, string>
            )[key]
          : sub,
      ] as [string, string, string],
  );
  return (
    <motion.section
      className="result"
      initial={reduced ? false : { opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
    >
      <div className="result-top">
        <div>
          <div className="eyebrow">
            {en ? "CHECK RESULTS" : "РЕЗУЛЬТАТ ПРОВЕРКИ"}
          </div>
          <h2>{result.hostname}</h2>
          <p>
            {en ? "Checked host: " : "Проверяемый хост: "}
            <b>{result.hostname}</b>
            <br />
            {en
              ? "Registration and purchase domain: "
              : "Домен регистрации и покупки: "}
            <b>{result.domain}</b>
          </p>
        </div>
        <button className="ghost" onClick={() => onRepeat(result.hostname)}>
          <span className="ghost-icon">↻</span>
          <span>{en ? "Check again" : "Проверить снова"}</span>
        </button>
      </div>
      <div className={"registration " + reg}>
        <span className="reg-orb" />
        <div>
          <small>{en ? "DOMAIN REGISTRATION" : "РЕГИСТРАЦИЯ ДОМЕНА"}</small>
          <strong>
            {en
              ? (
                  {
                    registered: "Registered",
                    unregistered: "Likely available",
                    available: "Available",
                    unknown: "Status unknown",
                  } as Record<string, string>
                )[reg]
              : labels[reg]}
          </strong>
          <p>{result.registration.detail}</p>
          <span className="meta">
            {result.registration.source} ·{" "}
            {new Date(result.registration.checkedAt).toLocaleString(
              en ? "en-US" : "ru-RU",
            )}
          </span>
        </div>
      </div>
      {(reg === "unregistered" || reg === "available") && (
        <section className="offers">
          <div className="section-heading">
            <div>
              <span className="eyebrow">
                {en ? "CHOOSE A REGISTRAR" : "ВЫБОР РЕГИСТРАТОРА"}
              </span>
              <h3>
                {en
                  ? `Where to buy ${result.domain}`
                  : `Где купить ${result.domain}`}
              </h3>
            </div>
            <p>
              {en
                ? "The registrar will confirm availability and the final price."
                : "Итоговую доступность и цену подтвердит регистратор."}
            </p>
          </div>
          <div className="offer-grid">
            {result.offers.map((o) => (
              <article className="offer" key={o.registrar}>
                <div className="offer-top">
                  <strong>{o.registrar}</strong>
                  {o.badge && <small>{o.badge}</small>}
                </div>
                <div className="price">
                  {o.firstYear != null ? (
                    <>
                      {o.kind === "exact" ? "" : en ? "from " : "от "}
                      {o.firstYear.toLocaleString("ru-RU")} {o.currency}
                    </>
                  ) : (
                    <span>
                      {en ? "Price to be confirmed" : "Цена уточняется"}
                    </span>
                  )}
                </div>
                <div className="offer-note">
                  {o.renewal != null
                    ? `Продление: ${o.renewal.toLocaleString("ru-RU")} ${o.currency}`
                    : en
                      ? "Rate not confirmed"
                      : "Тариф не подтверждён"}
                </div>
                {o.promo && (
                  <div className="promo-label">
                    {en ? "First-year offer" : "Акция первого года"}
                  </div>
                )}
                <a
                  href={o.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={() =>
                    trackRegistrarClick(
                      o.registrar,
                      result.domain,
                      o.firstYear != null
                        ? `${o.firstYear} ${o.currency}`
                        : undefined,
                      o.renewal != null
                        ? `${o.renewal} ${o.currency}`
                        : undefined,
                    )
                  }
                >
                  {en ? "Visit registrar ↗" : "На сайт регистратора ↗"}
                </a>
                <details>
                  <summary>
                    {en ? "Source and terms" : "Источник и условия"}
                  </summary>
                  <p>
                    {o.source}
                    {o.sourceDate && ` · ${o.sourceDate}`}
                    {o.stale && " · Тариф устарел"}
                    {o.conditions && ` · ${o.conditions}`}
                  </p>
                </details>
              </article>
            ))}
          </div>
        </section>
      )}
      <div
        className="tabs"
        role="tablist"
        aria-label={en ? "Diagnostics" : "Диагностика"}
      >
        <button
          role="tab"
          aria-selected={tab === "site"}
          className={tab === "site" ? "selected" : ""}
          onClick={() => setTab("site")}
        >
          {en ? "Website" : "Сайт"}
        </button>
        <button
          role="tab"
          aria-selected={tab === "mail"}
          className={tab === "mail" ? "selected" : ""}
          onClick={() => setTab("mail")}
        >
          {en ? "Email" : "Почта"}
        </button>
        <button
          role="tab"
          aria-selected={tab === "preview"}
          className={tab === "preview" ? "selected" : ""}
          onClick={() => setTab("preview")}
        >
          {en ? "Preview" : "Предпросмотр"}
        </button>
        <button
          role="tab"
          aria-selected={tab === "robots"}
          className={tab === "robots" ? "selected" : ""}
          onClick={() => setTab("robots")}
        >
          robots.txt
        </button>
        <button
          role="tab"
          aria-selected={tab === "sitemap"}
          className={tab === "sitemap" ? "selected" : ""}
          onClick={() => setTab("sitemap")}
        >
          sitemap.xml
        </button>
      </div>
      <AnimatePresence mode="wait">
        <motion.div
          key={tab}
          className="checks"
          role="tabpanel"
          initial={reduced ? false : { opacity: 0, x: 12 }}
          animate={{ opacity: 1, x: 0 }}
          exit={reduced ? {} : { opacity: 0, x: -12 }}
        >
          {tab === "preview" ? (
            <div className="preview-panel">
              <p className="preview-note">
                {en
                  ? "Approximation based on page metadata. Search engines and messengers may choose different text or images."
                  : "Приблизительный вид по метаданным страницы. Поисковики и мессенджеры могут выбрать другой текст или изображение."}
              </p>
              <div className="preview-grid">
                <article className="preview-card search-preview">
                  <small>{en ? "SEARCH RESULT" : "ПОИСКОВАЯ ВЫДАЧА"}</small>
                  <a
                    href={preview.url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {preview.title || result.hostname}
                  </a>
                  <span>{preview.url}</span>
                  <p>{preview.description}</p>
                </article>
                <article className="preview-card messenger-preview">
                  <small>
                    {en ? "MESSENGER LINK" : "ССЫЛКА В МЕССЕНДЖЕРЕ"}
                  </small>
                  {preview.image && (
                    <img
                      src={preview.image}
                      alt=""
                      referrerPolicy="no-referrer"
                    />
                  )}
                  <b>{preview.title || result.hostname}</b>
                  <p>{preview.description}</p>
                  <span>{preview.siteName || result.hostname}</span>
                </article>
              </div>
              <div className="preview-meta">
                {preview.summary} ·{" "}
                {preview.status === "ok"
                  ? en
                    ? "Metadata found"
                    : "Метаданные получены"
                  : en
                    ? "Metadata unavailable"
                    : "Метаданные недоступны"}
              </div>
            </div>
          ) : tab === "robots" || tab === "sitemap" ? (
            <div className="crawl-panel">
              <Card
                key={tab}
                title={tab === "robots" ? "robots.txt" : "sitemap.xml"}
                subtitle={
                  tab === "robots"
                    ? en
                      ? "Crawler access rules"
                      : "Правила доступа для роботов"
                    : en
                      ? "Site page list"
                      : "Список страниц сайта"
                }
                data={
                  crawlFinding || {
                    status: "unknown",
                    summary: en
                      ? "Not checked in this saved result"
                      : "В сохранённом результате не проверялось",
                    detail: "",
                    values: [],
                    checkedAt: result.checkedAt,
                  }
                }
                index={0}
                reduced={reduced}
                en={en}
              />
              {crawlFinding?.detail && (
                <p className="crawl-recommendation">
                  <b>{en ? "Recommendation: " : "Рекомендация: "}</b>
                  {crawlFinding.detail}
                </p>
              )}
            </div>
          ) : (
            items.map(([key, title, sub], i) => (
              <Card
                key={key}
                title={title}
                subtitle={sub}
                data={result[tab][key]}
                index={i}
                reduced={reduced}
                en={en}
              />
            ))
          )}
        </motion.div>
      </AnimatePresence>
      <div className="result-foot">
        {en ? "Checked: " : "Проверка: "}
        {new Date(result.checkedAt).toLocaleString(
          en ? "en-US" : "ru-RU",
        )} · {en ? "Region: " : "Регион: "}
        {result.region}{" "}
        {result.cached && (
          <span className="cache-tag">{en ? "Cached" : "Из кеша"}</span>
        )}
      </div>
    </motion.section>
  );
}
export default function App() {
  const [en, setEn] = useState(() => location.pathname.startsWith("/en"));
  const [input, setInput] = useState("");
  const [suggestions, setSuggestions] =
    useState<string[]>(getRandomSuggestions);
  const [result, setResult] = useState<Result | null>(null);
  const [page, setPage] = useState<"home" | "history">("home");
  const [historyPage, setHistoryPage] = useState(0);
  const resultRef = useRef<HTMLDivElement>(null);
  const startTimeRef = useRef<number>(0);
  const qc = useQueryClient();
  const reduced = !!useReducedMotion();

  useEffect(() => {
    initAnalytics();
  }, []);

  useEffect(() => {
    const currentPath =
      (en ? "/en" : "") + (page === "history" ? "#history" : "");
    trackPageView(currentPath, document.title);
  }, [page, en]);

  const switchLanguage = () => {
    const next = !en;
    setEn(next);
    trackLanguageSwitch(next ? "en" : "ru");
    document.documentElement.lang = next ? "en" : "ru";
    const path = location.pathname.replace(/^\/en(?=\/|$)/, "") || "/";
    window.history.replaceState(
      null,
      "",
      (next ? "/en" : "") + path + location.hash,
    );
  };

  useEffect(() => {
    document.documentElement.lang = en ? "en" : "ru";
    const title = en
      ? "Avari Domains — domain, website, DNS and email checker"
      : "Avari Domains — проверка домена, сайта, DNS и почты";
    const descriptionText = en
      ? "Check domain registration (WHOIS/RDAP), website availability, DNS, TLS and email security (SPF, DKIM, DMARC) in one report."
      : "Бесплатная комплексная проверка регистрации домена (WHOIS / RDAP), доступности сайта, DNS-записей, TLS-сертификатов, а также почтовых протоколов MX, SPF, DKIM и DMARC.";

    document.title = title;
    document
      .querySelector('meta[name="title"]')
      ?.setAttribute("content", title);
    document
      .querySelector('meta[name="description"]')
      ?.setAttribute("content", descriptionText);
    document
      .querySelector('meta[property="og:title"]')
      ?.setAttribute("content", title);
    document
      .querySelector('meta[property="og:description"]')
      ?.setAttribute("content", descriptionText);
    document
      .querySelector('meta[property="og:locale"]')
      ?.setAttribute("content", en ? "en_US" : "ru_RU");
    document
      .querySelector('link[rel="canonical"]')
      ?.setAttribute(
        "href",
        en ? "https://domains.avari.dev/en" : "https://domains.avari.dev/",
      );
  }, [en]);

  const history = useQuery({
    queryKey: ["history", historyPage],
    queryFn: () => api<History>("/history?page=" + historyPage),
    enabled: page === "history",
  });

  useEffect(() => {
    if (page === "history") {
      trackHistoryView(historyPage);
    }
  }, [page, historyPage]);

  const stats = useQuery({
    queryKey: ["stats"],
    queryFn: () => api<Stats>("/stats"),
    enabled: page === "history",
  });

  const check = useMutation({
    mutationFn: (value: string) => {
      startTimeRef.current = performance.now();
      return api<Result>("/check", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ input: value }),
      });
    },
    onSuccess: (r) => {
      const duration = Math.round(performance.now() - startTimeRef.current);
      trackDomainCheckSuccess(
        r.domain,
        r.registration.status,
        r.cached,
        duration,
      );
      setResult(r);
      setPage("home");
      qc.invalidateQueries({ queryKey: ["history"] });
      qc.invalidateQueries({ queryKey: ["stats"] });
    },
    onError: (err) => {
      const code =
        (err as unknown as ApiError | undefined)?.code || "unknown_error";
      trackDomainCheckError(input, code);
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (input.trim()) {
      trackDomainCheckStart(input.trim(), "input");
      check.mutate(input.trim());
    }
  };

  const repeat = (value: string) => {
    setInput(value);
    trackDomainCheckStart(value, "history");
    check.mutate(value);
  };

  const refreshSuggestions = () => {
    trackSuggestionRefresh();
    setSuggestions(getRandomSuggestions());
  };

  const handleSelectSuggestion = (value: string) => {
    setInput(value);
    trackSuggestionClick(value);
    trackDomainCheckStart(value, "suggestion");
    check.mutate(value);
  };

  const [showScrollTop, setShowScrollTop] = useState(false);
  const scrollToResult = (behavior: ScrollBehavior = "smooth") => {
    if (!resultRef.current) return;
    const rect = resultRef.current.getBoundingClientRect();
    const scrollTop = window.pageYOffset || document.documentElement.scrollTop;
    const targetY = Math.max(0, scrollTop + rect.top - 20);
    window.scrollTo({
      top: targetY,
      behavior,
    });
  };

  const scrollToTop = () => {
    trackScrollToTop();
    window.scrollTo({
      top: 0,
      behavior: reduced ? "instant" : "smooth",
    });
  };

  useEffect(() => {
    const onScroll = () => {
      setShowScrollTop(window.scrollY > 300);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    onScroll();
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    if (result && page === "home") {
      const raf = requestAnimationFrame(() => {
        scrollToResult(reduced ? "instant" : "smooth");
      });
      const timer = setTimeout(() => {
        scrollToResult(reduced ? "instant" : "smooth");
      }, 100);
      return () => {
        cancelAnimationFrame(raf);
        clearTimeout(timer);
      };
    }
  }, [result, page, reduced]);

  useEffect(() => {
    const onPop = () => {
      setPage(location.hash === "#history" ? "history" : "home");
    };
    window.addEventListener("hashchange", onPop);
    onPop();
    return () => window.removeEventListener("hashchange", onPop);
  }, []);
  return (
    <div className="app">
      <header>
        <div className="shell nav">
          <a className="brand" href="#home" onClick={() => setPage("home")}>
            <Logo />
            <span>
              AVARI<span>DOMAINS</span>
            </span>
          </a>
          <nav>
            <a
              href="#home"
              className={page === "home" ? "active" : ""}
              onClick={() => setPage("home")}
            >
              {en ? "Checker" : "Проверка"}
            </a>
            <a
              href="#history"
              className={page === "history" ? "active" : ""}
              onClick={() => setPage("history")}
            >
              {en ? "History" : "История"}
            </a>
            <button
              className="language-switch"
              onClick={switchLanguage}
              aria-label={en ? "Switch to Russian" : "Switch to English"}
            >
              {en ? "RU" : "EN"}
            </button>
          </nav>
        </div>
      </header>
      <main className="shell">
        <AnimatePresence mode="wait">
          {page === "home" ? (
            <motion.div
              key="home"
              initial={reduced ? false : { opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={reduced ? {} : { opacity: 0 }}
            >
              <section className="hero">
                <div className="hero-kicker">
                  <span className="pulse" />
                  {en
                    ? "REAL-TIME DOMAIN DIAGNOSTICS"
                    : "ДОМЕННАЯ ДИАГНОСТИКА В РЕАЛЬНОМ ВРЕМЕНИ"}
                </div>
                <h1>
                  {en ? "Check your domain," : "Проверь домен,"}
                  <br />
                  <em>{en ? "website and email." : "сайт и почту."}</em>
                </h1>
                <p className="hero-copy">
                  {en
                    ? "Registration, DNS, certificates, website availability and email settings in one report."
                    : "Регистрация, DNS, сертификат, доступность сайта и настройки почты — в одном отчёте."}
                </p>
                <form className="search" onSubmit={submit}>
                  <span className="search-icon">⌕</span>
                  <input
                    aria-label={en ? "Domain or URL" : "Домен или URL"}
                    placeholder={
                      en ? "Enter a domain or URL" : "Введите домен или URL"
                    }
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    maxLength={2048}
                  />
                  <button type="submit" disabled={check.isPending}>
                    {check.isPending
                      ? en
                        ? "Checking…"
                        : "Проверяем…"
                      : en
                        ? "Check →"
                        : "Проверить →"}
                  </button>
                </form>
                <div className="suggestions">
                  <span>{en ? "Try: " : "Попробуйте: "}</span>
                  {suggestions.map((x) => (
                    <button
                      key={x}
                      type="button"
                      onClick={() => handleSelectSuggestion(x)}
                    >
                      {x}
                    </button>
                  ))}
                  <button
                    type="button"
                    className="suggestions-refresh"
                    title={en ? "New suggestions" : "Другие варианты"}
                    aria-label={en ? "New suggestions" : "Другие варианты"}
                    onClick={refreshSuggestions}
                  >
                    ↻
                  </button>
                </div>
                {check.isPending && (
                  <div className="loading" role="status">
                    <span className="spinner" />
                    {en
                      ? "Checking registration, DNS, website and email. This may take up to 15 seconds."
                      : "Проверяем регистрацию, DNS, сайт и почту. Это может занять до 15 секунд."}
                  </div>
                )}
                {check.isError && (
                  <div className="error" role="alert">
                    {userError(check.error, en)}
                  </div>
                )}
              </section>
              {result && (
                <div ref={resultRef} style={{ scrollMarginTop: "24px" }}>
                  <Results
                    result={result}
                    onRepeat={repeat}
                    reduced={reduced}
                    en={en}
                  />
                </div>
              )}
              <section className="features">
                <div>
                  <span>{en ? "01 / DOMAIN" : "01 / ДОМЕН"}</span>
                  <h3>{en ? "Registration status" : "Статус регистрации"}</h3>
                  <p>
                    {en
                      ? "Check domain registration through RDAP or WHOIS."
                      : "Проверка регистрируемого домена через RDAP или WHOIS."}
                  </p>
                </div>
                <div>
                  <span>{en ? "02 / WEBSITE" : "02 / САЙТ"}</span>
                  <h3>{en ? "DNS and availability" : "DNS и доступность"}</h3>
                  <p>
                    {en
                      ? "Addresses, delegation, TLS and HTTP responses."
                      : "Адреса, делегирование, TLS и ответы HTTP."}
                  </p>
                </div>
                <div>
                  <span>{en ? "03 / EMAIL" : "03 / ПОЧТА"}</span>
                  <h3>{en ? "Email security" : "Почтовая защита"}</h3>
                  <p>
                    {en
                      ? "MX, SPF, DKIM and DMARC with clear explanations."
                      : "MX, SPF, DKIM и DMARC с понятными пояснениями."}
                  </p>
                </div>
              </section>
            </motion.div>
          ) : (
            <motion.section
              key="history"
              className="history"
              initial={reduced ? false : { opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={reduced ? {} : { opacity: 0 }}
            >
              <div className="eyebrow">
                {en ? "YOUR BROWSER" : "ВАШ БРАУЗЕР"}
              </div>
              <h1>{en ? "Check history" : "История проверок"}</h1>
              <p>
                {en
                  ? "Results are available only in this browser and are stored for 90 days."
                  : "Результаты доступны только в этом браузере и хранятся 90 дней."}
              </p>
              {stats.data && (
                <div className="stats">
                  <div>
                    <b>{stats.data.total}</b>
                    <span>{en ? "checks" : "проверок"}</span>
                  </div>
                  <div>
                    <b>{stats.data.registered}</b>
                    <span>{en ? "registered" : "зарегистрировано"}</span>
                  </div>
                  <div>
                    <b>{stats.data.likelyFree}</b>
                    <span>{en ? "likely available" : "вероятно свободны"}</span>
                  </div>
                </div>
              )}
              {history.isLoading ? (
                <div className="loading">
                  {en ? "Loading history…" : "Загружаем историю…"}
                </div>
              ) : history.isError ? (
                <div className="error">{userError(history.error, en)}</div>
              ) : history.data?.items.length ? (
                <>
                  <div className="history-list">
                    {history.data.items.map((x) => (
                      <article key={x.id}>
                        <div>
                          <strong>{x.hostname}</strong>
                          <span>
                            {en
                              ? (
                                  {
                                    registered: "Registered",
                                    unregistered: "Likely available",
                                    available: "Available",
                                    unknown: "Status unknown",
                                  } as Record<string, string>
                                )[x.registration.status]
                              : labels[x.registration.status]}{" "}
                            ·{" "}
                            {new Date(x.checkedAt).toLocaleString(
                              en ? "en-US" : "ru-RU",
                            )}
                          </span>
                        </div>
                        <div className="history-actions">
                          <button
                            onClick={() => {
                              trackHistoryItemClick(x.hostname);
                              setResult(x);
                              location.hash = "home";
                              setPage("home");
                            }}
                          >
                            {en ? "Open" : "Открыть"}
                          </button>
                          <button
                            onClick={() => {
                              trackHistoryItemClick(x.hostname);
                              repeat(x.hostname);
                            }}
                          >
                            {en ? "Check again ↗" : "Проверить снова ↗"}
                          </button>
                        </div>
                      </article>
                    ))}
                  </div>
                  <div className="pagination">
                    <button
                      disabled={historyPage === 0}
                      onClick={() => setHistoryPage((p) => p - 1)}
                    >
                      {en ? "← Previous" : "← Назад"}
                    </button>
                    <span>
                      {en
                        ? `Page ${historyPage + 1}`
                        : `Страница ${historyPage + 1}`}
                    </span>
                    <button
                      disabled={history.data.items.length < 20}
                      onClick={() => setHistoryPage((p) => p + 1)}
                    >
                      {en ? "Next →" : "Далее →"}
                    </button>
                  </div>
                </>
              ) : (
                <div className="empty">
                  {en
                    ? "No checks yet. Enter a domain to get started."
                    : "Проверок пока нет. Введите домен, чтобы начать."}
                </div>
              )}
            </motion.section>
          )}
        </AnimatePresence>
      </main>
      <footer>
        <div className="shell">
          <span>
            AVARI DOMAINS © {new Date().getFullYear()}{" "}
            <button className="language-switch" onClick={switchLanguage}>
              {en ? "RU" : "EN"}
            </button>
          </span>
          <span>
            {en
              ? "Diagnostic data may change. Purchases are made on the registrar’s website."
              : "Данные диагностики могут меняться. Покупка происходит на сайте регистратора."}
          </span>
        </div>
      </footer>
      <AnimatePresence>
        {showScrollTop && (
          <motion.button
            className="scroll-top-btn"
            onClick={scrollToTop}
            aria-label={en ? "Scroll to top" : "Наверх"}
            title={en ? "Scroll to top" : "Наверх"}
            initial={reduced ? false : { opacity: 0, scale: 0.8, y: 16 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={reduced ? {} : { opacity: 0, scale: 0.8, y: 16 }}
            transition={{ duration: 0.2 }}
          >
            <svg
              width="22"
              height="22"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <polyline points="18 15 12 9 6 15" />
            </svg>
          </motion.button>
        )}
      </AnimatePresence>
    </div>
  );
}
