import { useState, useEffect, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import type { Result, History, Stats, Finding, ApiError } from "./types";

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
}: {
  title: string;
  subtitle: string;
  data?: Finding;
  index: number;
  reduced: boolean;
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
            {statusText(data?.status || "unknown")}
          </span>
          <span className="chevron">⌄</span>
        </span>
      </button>
      <p>{data?.summary || "Ожидаем данные"}</p>
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
                Получено:{" "}
                {data?.checkedAt
                  ? new Date(data.checkedAt).toLocaleString("ru-RU")
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
}: {
  result: Result;
  onRepeat: (s: string) => void;
  reduced: boolean;
}) {
  const [tab, setTab] = useState<"site" | "mail">("site");
  const reg = result.registration.status;
  const items = tab === "site" ? siteItems : mailItems;
  return (
    <motion.section
      className="result"
      initial={reduced ? false : { opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
    >
      <div className="result-top">
        <div>
          <div className="eyebrow">РЕЗУЛЬТАТ ПРОВЕРКИ</div>
          <h2>{result.hostname}</h2>
          <p>
            Проверяемый хост: <b>{result.hostname}</b>
            <br />
            Домен регистрации и покупки: <b>{result.domain}</b>
          </p>
        </div>
        <button className="ghost" onClick={() => onRepeat(result.hostname)}>
          ↻ Проверить снова
        </button>
      </div>
      <div className={"registration " + reg}>
        <span className="reg-orb" />
        <div>
          <small>РЕГИСТРАЦИЯ ДОМЕНА</small>
          <strong>{labels[reg]}</strong>
          <p>{result.registration.detail}</p>
          <span className="meta">
            {result.registration.source} ·{" "}
            {new Date(result.registration.checkedAt).toLocaleString("ru-RU")}
          </span>
        </div>
      </div>
      <div className="tabs" role="tablist" aria-label="Диагностика">
        <button
          role="tab"
          aria-selected={tab === "site"}
          className={tab === "site" ? "selected" : ""}
          onClick={() => setTab("site")}
        >
          Сайт
        </button>
        <button
          role="tab"
          aria-selected={tab === "mail"}
          className={tab === "mail" ? "selected" : ""}
          onClick={() => setTab("mail")}
        >
          Почта
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
          {items.map(([key, title, sub], i) => (
            <Card
              key={key}
              title={title}
              subtitle={sub}
              data={result[tab][key]}
              index={i}
              reduced={reduced}
            />
          ))}
        </motion.div>
      </AnimatePresence>
      <div className="result-foot">
        Проверка: {new Date(result.checkedAt).toLocaleString("ru-RU")} · Регион:{" "}
        {result.region}{" "}
        {result.cached && <span className="cache-tag">Из кеша</span>}
      </div>
      {(reg === "unregistered" || reg === "available") && (
        <section className="offers">
          <div className="section-heading">
            <div>
              <span className="eyebrow">ВЫБОР РЕГИСТРАТОРА</span>
              <h3>Где купить {result.domain}</h3>
            </div>
            <p>Итоговую доступность и цену подтвердит регистратор.</p>
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
                      {o.kind === "exact" ? "" : "от "}
                      {o.firstYear.toLocaleString("ru-RU")} {o.currency}
                    </>
                  ) : (
                    <span>Цена уточняется</span>
                  )}
                </div>
                <div className="offer-note">
                  {o.renewal != null
                    ? `Продление: ${o.renewal.toLocaleString("ru-RU")} ${o.currency}`
                    : "Тариф не подтверждён"}
                </div>
                {o.promo && (
                  <div className="promo-label">Акция первого года</div>
                )}
                <a href={o.url} target="_blank" rel="noopener noreferrer">
                  На сайт регистратора ↗
                </a>
                <details>
                  <summary>Источник и условия</summary>
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
    </motion.section>
  );
}
export default function App() {
  const [input, setInput] = useState("");
  const [result, setResult] = useState<Result | null>(null);
  const [page, setPage] = useState<"home" | "history">("home");
  const [historyPage, setHistoryPage] = useState(0);
  const qc = useQueryClient();
  const reduced = !!useReducedMotion();
  const history = useQuery({
    queryKey: ["history", historyPage],
    queryFn: () => api<History>("/history?page=" + historyPage),
    enabled: page === "history",
  });
  const stats = useQuery({
    queryKey: ["stats"],
    queryFn: () => api<Stats>("/stats"),
    enabled: page === "history",
  });
  const check = useMutation({
    mutationFn: (value: string) =>
      api<Result>("/check", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ input: value }),
      }),
    onSuccess: (r) => {
      setResult(r);
      setPage("home");
      qc.invalidateQueries({ queryKey: ["history"] });
      qc.invalidateQueries({ queryKey: ["stats"] });
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (input.trim()) check.mutate(input.trim());
  };
  const repeat = (value: string) => {
    setInput(value);
    check.mutate(value);
    window.scrollTo({ top: 0, behavior: reduced ? "instant" : "smooth" });
  };
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
              Проверка
            </a>
            <a
              href="#history"
              className={page === "history" ? "active" : ""}
              onClick={() => setPage("history")}
            >
              История
            </a>
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
                  ДОМЕННАЯ ДИАГНОСТИКА В РЕАЛЬНОМ ВРЕМЕНИ
                </div>
                <h1>
                  Проверь домен,
                  <br />
                  <em>сайт и почту.</em>
                </h1>
                <p className="hero-copy">
                  Регистрация, DNS, сертификат, доступность сайта и настройки
                  почты — в одном отчёте.
                </p>
                <form className="search" onSubmit={submit}>
                  <span className="search-icon">⌕</span>
                  <input
                    aria-label="Домен или URL"
                    placeholder="Введите домен или URL"
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    maxLength={2048}
                  />
                  <button type="submit" disabled={check.isPending}>
                    {check.isPending ? "Проверяем…" : "Проверить →"}
                  </button>
                </form>
                <div className="suggestions">
                  Попробуйте:{" "}
                  {["example.com", "яндекс.рф", "www.github.com"].map((x) => (
                    <button key={x} onClick={() => setInput(x)}>
                      {x}
                    </button>
                  ))}
                </div>
                {check.isPending && (
                  <div className="loading" role="status">
                    <span className="spinner" />
                    Проверяем регистрацию, DNS, сайт и почту. Это может занять
                    до 15 секунд.
                  </div>
                )}
                {check.isError && (
                  <div className="error" role="alert">
                    {(check.error as unknown as ApiError).message ||
                      "Не удалось выполнить проверку"}
                  </div>
                )}
              </section>
              {result && (
                <Results result={result} onRepeat={repeat} reduced={reduced} />
              )}
              <section className="features">
                <div>
                  <span>01 / ДОМЕН</span>
                  <h3>Статус регистрации</h3>
                  <p>Проверка регистрируемого домена через RDAP или WHOIS.</p>
                </div>
                <div>
                  <span>02 / САЙТ</span>
                  <h3>DNS и доступность</h3>
                  <p>Адреса, делегирование, TLS и ответы HTTP.</p>
                </div>
                <div>
                  <span>03 / ПОЧТА</span>
                  <h3>Почтовая защита</h3>
                  <p>MX, SPF, DKIM и DMARC с понятными пояснениями.</p>
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
              <div className="eyebrow">ВАШ БРАУЗЕР</div>
              <h1>История проверок</h1>
              <p>
                Результаты доступны только в этом браузере и хранятся 90 дней.
              </p>
              {stats.data && (
                <div className="stats">
                  <div>
                    <b>{stats.data.total}</b>
                    <span>проверок</span>
                  </div>
                  <div>
                    <b>{stats.data.registered}</b>
                    <span>зарегистрировано</span>
                  </div>
                  <div>
                    <b>{stats.data.likelyFree}</b>
                    <span>вероятно свободны</span>
                  </div>
                </div>
              )}
              {history.isLoading ? (
                <div className="loading">Загружаем историю…</div>
              ) : history.isError ? (
                <div className="error">Не удалось загрузить историю</div>
              ) : history.data?.items.length ? (
                <>
                  <div className="history-list">
                    {history.data.items.map((x) => (
                      <article key={x.id}>
                        <div>
                          <strong>{x.hostname}</strong>
                          <span>
                            {labels[x.registration.status]} ·{" "}
                            {new Date(x.checkedAt).toLocaleString("ru-RU")}
                          </span>
                        </div>
                        <div className="history-actions">
                          <button
                            onClick={() => {
                              setResult(x);
                              location.hash = "home";
                              setPage("home");
                            }}
                          >
                            Открыть
                          </button>
                          <button onClick={() => repeat(x.hostname)}>
                            Проверить снова ↗
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
                      ← Назад
                    </button>
                    <span>Страница {historyPage + 1}</span>
                    <button
                      disabled={history.data.items.length < 20}
                      onClick={() => setHistoryPage((p) => p + 1)}
                    >
                      Далее →
                    </button>
                  </div>
                </>
              ) : (
                <div className="empty">
                  Проверок пока нет. Введите домен, чтобы начать.
                </div>
              )}
            </motion.section>
          )}
        </AnimatePresence>
      </main>
      <footer>
        <div className="shell">
          <span>AVARI DOMAINS © {new Date().getFullYear()}</span>
          <span>
            Данные диагностики могут меняться. Покупка происходит на сайте
            регистратора.
          </span>
        </div>
      </footer>
    </div>
  );
}
