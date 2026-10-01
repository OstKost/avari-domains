export type Finding = {
  status: "ok" | "warning" | "missing" | "neutral" | "unknown";
  summary: string;
  values?: string[];
  detail?: string;
  checkedAt: string;
};
export type Registration = {
  status: "registered" | "unregistered" | "available" | "unknown";
  source: string;
  detail: string;
  checkedAt: string;
};
export type Offer = {
  registrar: string;
  url: string;
  firstYear?: number;
  renewal?: number;
  currency: string;
  kind: string;
  source: string;
  sourceDate: string;
  stale: boolean;
  badge?: string;
  promo: boolean;
  conditions?: string;
};
export type Result = {
  id: string;
  input: string;
  hostname: string;
  domain: string;
  registration: Registration;
  site: Record<string, Finding>;
  mail: Record<string, Finding>;
  offers: Offer[];
  preview: {
    title: string;
    description: string;
    image: string;
    siteName: string;
    url: string;
    status: string;
    summary: string;
  };
  robots?: Finding;
  sitemap?: Finding;
  checkedAt: string;
  region: string;
  cached: boolean;
};
export type History = { items: Result[]; page: number; pageSize: number };
export type Stats = { total: number; registered: number; likelyFree: number };
export type ApiError = { code: string; message: string };
